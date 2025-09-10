package database

import (
	"context"	
	"fmt"
	"log"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/dbresolver"
)

// GetDB 获取数据库实例
// 返回配置了主从分离的GORM数据库实例
func (m *DBManager) GetDB() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db
}

// GetMasterDB 获取主库实例
// 强制使用主库进行读写操作
func (m *DBManager) GetMasterDB() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db.Clauses(dbresolver.Write)
}

// GetSlaveDB 获取从库实例
// 强制使用从库进行只读操作
func (m *DBManager) GetSlaveDB() *gorm.DB {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.db.Clauses(dbresolver.Read)
}

// Transaction 执行事务
// 参数:
//   - ctx: 上下文
//   - fn: 事务执行函数
// 返回值:
//   - error: 错误信息
func (m *DBManager) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	if fn == nil {
		return fmt.Errorf("transaction function cannot be nil")
	}

	// 使用主库执行事务
	tx := m.GetMasterDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// HealthCheck 健康检查
// 检查主库和所有从库的连接状态
// 参数:
//   - ctx: 上下文
// 返回值:
//   - map[string]HealthStatus: 各数据库的健康状态
func (m *DBManager) HealthCheck(ctx context.Context) map[string]HealthStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make(map[string]HealthStatus)

	// 检查主库
	result["master"] = m.checkSingleDB(ctx, m.db, "master")

	// 检查从库
	for i, slaveConfig := range m.config.Slaves {
		key := fmt.Sprintf("slave_%d", i)
		// 这里简化处理，实际应该获取具体的从库连接
		result[key] = m.checkSingleDB(ctx, m.db, slaveConfig.DSN)
	}

	m.lastHealthCheck = time.Now()
	return result
}

// checkSingleDB 检查单个数据库的健康状态
// 参数:
//   - ctx: 上下文
//   - db: 数据库实例
//   - name: 数据库名称
// 返回值:
//   - HealthStatus: 健康状态
func (m *DBManager) checkSingleDB(ctx context.Context, db *gorm.DB, name string) HealthStatus {
	start := time.Now()
	status := HealthStatus{
		LastCheckTime: start,
		IsHealthy:     false,
	}

	// 设置超时上下文
	timeoutCtx, cancel := context.WithTimeout(ctx, m.config.MonitorConfig.ConnectionTimeout)
	defer cancel()

	// 执行ping操作
	sqlDB, err := db.DB()
	if err != nil {
		status.ErrorMessage = fmt.Sprintf("failed to get sql.DB: %v", err)
		status.ResponseTime = time.Since(start)
		return status
	}

	err = sqlDB.PingContext(timeoutCtx)
	status.ResponseTime = time.Since(start)

	if err != nil {
		status.ErrorMessage = fmt.Sprintf("ping failed: %v", err)
	} else {
		status.IsHealthy = true
	}

	return status
}

// GetStats 获取数据库统计信息
// 返回主库和从库的连接池统计信息
// 返回值:
//   - map[string]DatabaseStats: 各数据库的统计信息
func (m *DBManager) GetStats() map[string]DatabaseStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]DatabaseStats)

	// 获取主库统计信息
	if sqlDB, err := m.db.DB(); err == nil {
		stats := sqlDB.Stats()
		result["master"] = DatabaseStats{
			OpenConnections:   stats.OpenConnections,
			InUse:             stats.InUse,
			Idle:              stats.Idle,
			WaitCount:         stats.WaitCount,
			WaitDuration:      stats.WaitDuration,
			MaxIdleClosed:     stats.MaxIdleClosed,
			MaxIdleTimeClosed: stats.MaxIdleTimeClosed,
			MaxLifetimeClosed: stats.MaxLifetimeClosed,
		}
	}

	// 这里简化处理，实际应该分别获取各从库的统计信息
	for i := range m.config.Slaves {
		key := fmt.Sprintf("slave_%d", i)
		result[key] = result["master"] // 简化处理
	}

	return result
}

// Close 关闭数据库连接
// 优雅关闭所有数据库连接和监控协程
// 返回值:
//   - error: 错误信息
func (m *DBManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 取消上下文，停止监控
	m.cancel()

	// 等待所有协程结束
	m.wg.Wait()

	// 关闭数据库连接
	if m.db != nil {
		if sqlDB, err := m.db.DB(); err == nil {
			return sqlDB.Close()
		}
	}

	return nil
}

// Ping 测试数据库连接
// 测试主库连接是否正常
// 参数:
//   - ctx: 上下文
// 返回值:
//   - error: 错误信息
func (m *DBManager) Ping(ctx context.Context) error {
	sqlDB, err := m.db.DB()
	if err != nil {
		return fmt.Errorf("failed to get sql.DB: %w", err)
	}

	return sqlDB.PingContext(ctx)
}

// initDB 初始化数据库连接
// 配置主从分离、连接池和日志
// 返回值:
//   - error: 错误信息
func (m *DBManager) initDB() error {
	// 根据数据库类型选择驱动
	dialector, err := m.getDialector(m.config.Master, m.config.Type)
	if err != nil {
		return fmt.Errorf("failed to get dialector for master: %w", err)
	}

	// 配置GORM
	gormConfig := &gorm.Config{
		Logger: m.createGormLogger(),
	}

	// 打开数据库连接
	m.db, err = gorm.Open(dialector, gormConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to master database: %w", err)
	}

	// 配置连接池
	if err := m.configureConnectionPool(m.db, m.config.PoolConfig); err != nil {
		return fmt.Errorf("failed to configure master connection pool: %w", err)
	}

	// 配置主从分离
	if len(m.config.Slaves) > 0 {
		if err := m.configureDBResolver(); err != nil {
			return fmt.Errorf("failed to configure db resolver: %w", err)
		}
	}

	return nil
}

// getDialector 根据数据库类型获取对应的方言
// 参数:
//   - dsn: 数据源名称
//   - dbType: 数据库类型
// 返回值:
//   - gorm.Dialector: GORM方言
//   - error: 错误信息
func (m *DBManager) getDialector(dsn, dbType string) (gorm.Dialector, error) {
	switch dbType {
	case "mysql":
		return mysql.Open(dsn), nil
	case "postgres", "postgresql":
		return postgres.Open(dsn), nil
	case "sqlite", "sqlite3":
		return sqlite.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// configureConnectionPool 配置连接池
// 参数:
//   - db: 数据库实例
//   - config: 连接池配置
// 返回值:
//   - error: 错误信息
func (m *DBManager) configureConnectionPool(db *gorm.DB, config PoolConfig) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	// 设置连接池参数
	if config.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(config.MaxOpenConns)
	}

	if config.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(config.MaxIdleConns)
	}

	if config.ConnMaxLifetime > 0 {
		sqlDB.SetConnMaxLifetime(config.ConnMaxLifetime)
	}

	if config.ConnMaxIdleTime > 0 {
		sqlDB.SetConnMaxIdleTime(config.ConnMaxIdleTime)
	}

	return nil
}

// configureDBResolver 配置数据库解析器（主从分离）
// 返回值:
//   - error: 错误信息
func (m *DBManager) configureDBResolver() error {
	// 准备从库配置
	var replicas []gorm.Dialector
	var sources []gorm.Dialector

	for _, slaveConfig := range m.config.Slaves {
		dialector, err := m.getDialector(slaveConfig.DSN, slaveConfig.Type)
		if err != nil {
			return fmt.Errorf("failed to get dialector for slave %s: %w", slaveConfig.DSN, err)
		}
		replicas = append(replicas, dialector)
	}

	// 主库也作为源
	masterDialector, err := m.getDialector(m.config.Master, m.config.Type)
	if err != nil {
		return fmt.Errorf("failed to get dialector for master: %w", err)
	}
	sources = append(sources, masterDialector)

	// 配置DBResolver插件
	resolverConfig := dbresolver.Config{
		Sources:  sources,
		Replicas: replicas,
		Policy:   dbresolver.RandomPolicy{}, // 随机策略
	}

	// 为每个从库配置连接池
	for range m.config.Slaves {
		resolverConfig.TraceResolverMode = true
	}

	return m.db.Use(dbresolver.Register(resolverConfig))
}

// createGormLogger 创建GORM日志记录器
// 返回值:
//   - logger.Interface: GORM日志接口
func (m *DBManager) createGormLogger() logger.Interface {
	if !m.config.LogConfig.Enabled {
		return logger.Discard
	}

	// 设置日志级别
	var logLevel logger.LogLevel
	switch m.config.LogConfig.Level {
	case "silent":
		logLevel = logger.Silent
	case "error":
		logLevel = logger.Error
	case "warn":
		logLevel = logger.Warn
	case "info":
		logLevel = logger.Info
	default:
		logLevel = logger.Info
	}

	// 创建自定义日志记录器
	return logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             m.config.SlowQueryConfig.Threshold,
			LogLevel:                  logLevel,
			IgnoreRecordNotFoundError: m.config.LogConfig.IgnoreRecordNotFoundError,
			ParameterizedQueries:      m.config.LogConfig.ParameterizedQueries,
			Colorful:                  m.config.LogConfig.Colorful,
		},
	)
}

// startMonitoring 启动监控协程
// 定期执行健康检查
func (m *DBManager) startMonitoring() {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()

		ticker := time.NewTicker(m.config.MonitorConfig.HealthCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-m.ctx.Done():
				return
			case <-ticker.C:
				// 执行健康检查
				ctx, cancel := context.WithTimeout(context.Background(), m.config.MonitorConfig.ConnectionTimeout)
				status := m.HealthCheck(ctx)
				cancel()

				// 记录不健康的数据库
				for name, health := range status {
					if !health.IsHealthy {
						m.logger.Error(m.ctx, "Database health check failed", "database", name, "error", health.ErrorMessage)
					}
				}
			}
		}
	}()
}

// newDefaultLogger 创建默认日志记录器
// 返回值:
//   - Logger: 日志记录器接口
func (m *DBManager) newDefaultLogger() Logger {
	return &DefaultLogger{
		config: m.config.LogConfig,
		logger: log.New(os.Stdout, "[DATABASE] ", log.LstdFlags),
	}
}

// newSlowQueryLogger 创建慢查询日志记录器
// 返回值:
//   - Logger: 慢查询日志记录器
func (m *DBManager) newSlowQueryLogger() Logger {
	return &SlowQueryLogger{
		config:     m.config.SlowQueryConfig,
		baseLogger: m.logger,
		logger:     log.New(os.Stdout, "[SLOW_QUERY] ", log.LstdFlags),
	}
}