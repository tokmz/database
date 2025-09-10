package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// User 示例用户模型
type User struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	Email     string    `gorm:"size:100;uniqueIndex" json:"email"`
	Age       int       `json:"age"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	DeletedAt *time.Time `gorm:"index" json:"deleted_at,omitempty"`
}

// ExampleUsage 展示数据库管理器的使用方法
func ExampleUsage() {
	// 1. 创建数据库配置
	config := &Config{
		Master: "root:password@tcp(localhost:3306)/test_db?charset=utf8mb4&parseTime=True&loc=Local",
		Type:   "mysql",
		Slaves: []SlaveConfig{
			{
				DSN:    "root:password@tcp(localhost:3307)/test_db?charset=utf8mb4&parseTime=True&loc=Local",
				Type:   "mysql",
				Weight: 1,
				PoolConfig: PoolConfig{
					MaxOpenConns:    10,
					MaxIdleConns:    5,
					ConnMaxLifetime: time.Hour,
					ConnMaxIdleTime: 30 * time.Minute,
				},
			},
		},
		PoolConfig: PoolConfig{
			MaxOpenConns:    20,
			MaxIdleConns:    10,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 30 * time.Minute,
		},
		LogConfig: LogConfig{
			Enabled:                   true,
			Level:                     "info",
			Colorful:                  true,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		},
		SlowQueryConfig: SlowQueryConfig{
			Enabled:   true,
			Threshold: 200 * time.Millisecond,
			LogParams: true,
		},
		MonitorConfig: MonitorConfig{
			Enabled:             true,
			HealthCheckInterval: 30 * time.Second,
			ConnectionTimeout:   5 * time.Second,
			MaxRetries:          3,
		},
	}

	// 2. 创建数据库管理器
	manager, err := NewManager(config)
	if err != nil {
		fmt.Printf("创建数据库管理器失败: %v\n", err)
		return
	}
	defer manager.Close()

	// 3. 获取数据库实例
	db := manager.GetDB()

	// 4. 自动迁移表结构
	err = db.AutoMigrate(&User{})
	if err != nil {
		fmt.Printf("表迁移失败: %v\n", err)
		return
	}

	// 5. 基本CRUD操作示例
	exampleCRUD(manager)

	// 6. 事务操作示例
	exampleTransaction(manager)

	// 7. 主从分离示例
	exampleMasterSlave(manager)

	// 8. 健康检查示例
	exampleHealthCheck(manager)

	// 9. 统计信息示例
	exampleStats(manager)
}

// exampleCRUD 演示基本的CRUD操作
func exampleCRUD(manager Manager) {
	fmt.Println("=== CRUD操作示例 ===")

	db := manager.GetDB()
	ctx := context.Background()

	// 创建用户
	user := &User{
		Name:  "张三",
		Email: "zhangsan@example.com",
		Age:   25,
	}

	result := db.WithContext(ctx).Create(user)
	if result.Error != nil {
		fmt.Printf("创建用户失败: %v\n", result.Error)
		return
	}
	fmt.Printf("创建用户成功，ID: %d\n", user.ID)

	// 查询用户
	var foundUser User
	err := db.WithContext(ctx).First(&foundUser, user.ID).Error
	if err != nil {
		fmt.Printf("查询用户失败: %v\n", err)
		return
	}
	fmt.Printf("查询用户成功: %+v\n", foundUser)

	// 更新用户
	err = db.WithContext(ctx).Model(&foundUser).Update("age", 26).Error
	if err != nil {
		fmt.Printf("更新用户失败: %v\n", err)
		return
	}
	fmt.Println("更新用户成功")

	// 删除用户（软删除）
	err = db.WithContext(ctx).Delete(&foundUser).Error
	if err != nil {
		fmt.Printf("删除用户失败: %v\n", err)
		return
	}
	fmt.Println("删除用户成功")
}

// exampleTransaction 演示事务操作
func exampleTransaction(manager Manager) {
	fmt.Println("\n=== 事务操作示例 ===")

	ctx := context.Background()

	// 执行事务
	err := manager.Transaction(ctx, func(tx *gorm.DB) error {
		// 在事务中创建多个用户
		users := []User{
			{Name: "李四", Email: "lisi@example.com", Age: 30},
			{Name: "王五", Email: "wangwu@example.com", Age: 28},
		}

		for _, user := range users {
			if err := tx.Create(&user).Error; err != nil {
				return fmt.Errorf("创建用户失败: %w", err)
			}
			fmt.Printf("事务中创建用户: %s\n", user.Name)
		}

		return nil
	})

	if err != nil {
		fmt.Printf("事务执行失败: %v\n", err)
	} else {
		fmt.Println("事务执行成功")
	}
}

// exampleMasterSlave 演示主从分离操作
func exampleMasterSlave(manager Manager) {
	fmt.Println("\n=== 主从分离示例 ===")

	ctx := context.Background()

	// 强制使用主库进行写操作
	masterDB := manager.GetMasterDB()
	user := &User{
		Name:  "赵六",
		Email: "zhaoliu@example.com",
		Age:   35,
	}

	err := masterDB.WithContext(ctx).Create(user).Error
	if err != nil {
		fmt.Printf("主库写入失败: %v\n", err)
	} else {
		fmt.Printf("主库写入成功，用户ID: %d\n", user.ID)
	}

	// 强制使用从库进行读操作
	slaveDB := manager.GetSlaveDB()
	var users []User
	err = slaveDB.WithContext(ctx).Find(&users).Error
	if err != nil {
		fmt.Printf("从库读取失败: %v\n", err)
	} else {
		fmt.Printf("从库读取成功，用户数量: %d\n", len(users))
	}
}

// exampleHealthCheck 演示健康检查
func exampleHealthCheck(manager Manager) {
	fmt.Println("\n=== 健康检查示例 ===")

	ctx := context.Background()

	// 执行健康检查
	healthStatus := manager.HealthCheck(ctx)

	for dbName, status := range healthStatus {
		if status.IsHealthy {
			fmt.Printf("%s: 健康 (响应时间: %v)\n", dbName, status.ResponseTime)
		} else {
			fmt.Printf("%s: 不健康 - %s\n", dbName, status.ErrorMessage)
		}
	}

	// 测试连接
	err := manager.Ping(ctx)
	if err != nil {
		fmt.Printf("数据库连接测试失败: %v\n", err)
	} else {
		fmt.Println("数据库连接测试成功")
	}
}

// exampleStats 演示统计信息获取
func exampleStats(manager Manager) {
	fmt.Println("\n=== 统计信息示例 ===")

	// 获取数据库统计信息
	stats := manager.GetStats()

	for dbName, stat := range stats {
		fmt.Printf("%s 连接池统计:\n", dbName)
		fmt.Printf("  打开连接数: %d\n", stat.OpenConnections)
		fmt.Printf("  使用中连接数: %d\n", stat.InUse)
		fmt.Printf("  空闲连接数: %d\n", stat.Idle)
		fmt.Printf("  等待次数: %d\n", stat.WaitCount)
		fmt.Printf("  等待时间: %v\n", stat.WaitDuration)
		fmt.Println()
	}
}

// ExampleWithCustomLogger 演示使用自定义日志记录器
func ExampleWithCustomLogger() {
	fmt.Println("\n=== 自定义日志记录器示例 ===")

	// 创建自定义日志记录器
	customLogger := NewZapLogger(nil) // 这里应该传入真实的zap logger

	// 创建配置
	config := &Config{
		Master: "root:password@tcp(localhost:3306)/test_db?charset=utf8mb4&parseTime=True&loc=Local",
		Type:   "mysql",
		PoolConfig: PoolConfig{
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: 30 * time.Minute,
		},
		LogConfig: LogConfig{
			Enabled: true,
			Level:   "info",
		},
	}

	// 使用自定义日志记录器创建管理器
	manager, err := NewManager(config, customLogger)
	if err != nil {
		fmt.Printf("创建数据库管理器失败: %v\n", err)
		return
	}
	defer manager.Close()

	fmt.Println("使用自定义日志记录器的数据库管理器创建成功")
}

// ExampleConfigValidation 演示配置验证
func ExampleConfigValidation() {
	fmt.Println("\n=== 配置验证示例 ===")

	// 无效配置示例
	invalidConfigs := []*Config{
		{
			// 缺少Master DSN
			Type: "mysql",
		},
		{
			Master: "invalid_dsn",
			// 缺少Type
		},
		{
			Master: "root:password@tcp(localhost:3306)/test_db",
			Type:   "mysql",
			PoolConfig: PoolConfig{
				MaxOpenConns: -1, // 无效值
			},
		},
	}

	for i, config := range invalidConfigs {
		_, err := NewManager(config)
		if err != nil {
			fmt.Printf("配置 %d 验证失败（预期）: %v\n", i+1, err)
		} else {
			fmt.Printf("配置 %d 验证通过（意外）\n", i+1)
		}
	}
}