package database

import (
	"context"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Logger 日志接口，支持扩展不同的日志实现
// 可以适配zap、logrus等第三方日志库
type Logger interface {
	// LogMode 设置日志模式
	LogMode(level LogLevel) Logger
	// Info 记录信息级别日志
	Info(ctx context.Context, msg string, data ...interface{})
	// Warn 记录警告级别日志
	Warn(ctx context.Context, msg string, data ...interface{})
	// Error 记录错误级别日志
	Error(ctx context.Context, msg string, data ...interface{})
	// Trace 记录SQL执行轨迹
	Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error)
}

// LogLevel 日志级别枚举
type LogLevel int

const (
	// Silent 静默模式，不输出任何日志
	Silent LogLevel = iota + 1
	// Error 只输出错误日志
	Error
	// Warn 输出警告和错误日志
	Warn
	// Info 输出所有级别日志
	Info
)

// HealthStatus 数据库健康状态
type HealthStatus struct {
	// IsHealthy 是否健康
	IsHealthy bool `json:"is_healthy"`
	// LastCheckTime 最后检查时间
	LastCheckTime time.Time `json:"last_check_time"`
	// ErrorMessage 错误信息
	ErrorMessage string `json:"error_message,omitempty"`
	// ResponseTime 响应时间
	ResponseTime time.Duration `json:"response_time"`
}

// DatabaseStats 数据库统计信息
type DatabaseStats struct {
	// OpenConnections 当前打开的连接数
	OpenConnections int `json:"open_connections"`
	// InUse 正在使用的连接数
	InUse int `json:"in_use"`
	// Idle 空闲连接数
	Idle int `json:"idle"`
	// WaitCount 等待连接的总次数
	WaitCount int64 `json:"wait_count"`
	// WaitDuration 等待连接的总时间
	WaitDuration time.Duration `json:"wait_duration"`
	// MaxIdleClosed 因超过最大空闲时间而关闭的连接数
	MaxIdleClosed int64 `json:"max_idle_closed"`
	// MaxIdleTimeClosed 因超过最大空闲时间而关闭的连接数
	MaxIdleTimeClosed int64 `json:"max_idle_time_closed"`
	// MaxLifetimeClosed 因超过最大生命周期而关闭的连接数
	MaxLifetimeClosed int64 `json:"max_lifetime_closed"`
}

// Manager 数据库管理器接口
// 定义了数据库操作的核心方法
type Manager interface {
	// GetDB 获取数据库实例
	GetDB() *gorm.DB
	// GetMasterDB 获取主库实例
	GetMasterDB() *gorm.DB
	// GetSlaveDB 获取从库实例
	GetSlaveDB() *gorm.DB
	// Transaction 执行事务
	Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error
	// HealthCheck 健康检查
	HealthCheck(ctx context.Context) map[string]HealthStatus
	// GetStats 获取数据库统计信息
	GetStats() map[string]DatabaseStats
	// Close 关闭数据库连接
	Close() error
	// Ping 测试数据库连接
	Ping(ctx context.Context) error
}

// DBManager 数据库管理器实现
type DBManager struct {
	// config 数据库配置
	config *Config
	// db GORM数据库实例
	db *gorm.DB
	// logger 日志记录器
	logger Logger
	// mu 读写锁，保护并发访问
	mu sync.RWMutex
	// healthStatus 健康状态缓存
	healthStatus map[string]HealthStatus
	// lastHealthCheck 最后健康检查时间
	lastHealthCheck time.Time
	// slowQueryLogger 慢查询日志记录器
	slowQueryLogger Logger
	// ctx 上下文
	ctx context.Context
	// cancel 取消函数
	cancel context.CancelFunc
	// wg 等待组，用于优雅关闭
	wg sync.WaitGroup
}

// NewManager 创建新的数据库管理器实例
// 参数:
//   - config: 数据库配置
//   - logger: 日志记录器，可选参数
// 返回值:
//   - Manager: 数据库管理器接口
//   - error: 错误信息
func NewManager(config *Config, logger ...Logger) (Manager, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// 验证配置
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &DBManager{
		config:       config,
		healthStatus: make(map[string]HealthStatus),
		ctx:          ctx,
		cancel:       cancel,
	}

	// 设置日志记录器
	if len(logger) > 0 && logger[0] != nil {
		manager.logger = logger[0]
	} else {
		// 使用默认日志记录器
		manager.logger = manager.newDefaultLogger()
	}

	// 设置慢查询日志记录器
	manager.slowQueryLogger = manager.newSlowQueryLogger()

	// 初始化数据库连接
	if err := manager.initDB(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// 启动监控
	if config.MonitorConfig.Enabled {
		manager.startMonitoring()
	}

	return manager, nil
}

// validateConfig 验证配置的有效性
// 参数:
//   - config: 数据库配置
// 返回值:
//   - error: 验证错误信息
func validateConfig(config *Config) error {
	if config.Master == "" {
		return fmt.Errorf("master database DSN cannot be empty")
	}

	if config.Type == "" {
		return fmt.Errorf("database type cannot be empty")
	}

	// 验证连接池配置
	if config.PoolConfig.MaxOpenConns < 0 {
		return fmt.Errorf("max open connections cannot be negative")
	}

	if config.PoolConfig.MaxIdleConns < 0 {
		return fmt.Errorf("max idle connections cannot be negative")
	}

	if config.PoolConfig.MaxIdleConns > config.PoolConfig.MaxOpenConns && config.PoolConfig.MaxOpenConns > 0 {
		return fmt.Errorf("max idle connections cannot be greater than max open connections")
	}

	// 验证慢查询配置
	if config.SlowQueryConfig.Enabled && config.SlowQueryConfig.Threshold <= 0 {
		return fmt.Errorf("slow query threshold must be positive when enabled")
	}

	// 验证监控配置
	if config.MonitorConfig.Enabled {
		if config.MonitorConfig.HealthCheckInterval <= 0 {
			return fmt.Errorf("health check interval must be positive when monitoring is enabled")
		}
		if config.MonitorConfig.ConnectionTimeout <= 0 {
			return fmt.Errorf("connection timeout must be positive when monitoring is enabled")
		}
		if config.MonitorConfig.MaxRetries < 0 {
			return fmt.Errorf("max retries cannot be negative")
		}
	}

	return nil
}