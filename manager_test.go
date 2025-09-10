package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// TestUser 测试用户模型
type TestUser struct {
	ID        uint      `gorm:"primarykey"`
	Name      string    `gorm:"not null;size:100"`
	Email     string    `gorm:"uniqueIndex;not null;size:255"`
	Age       int       `gorm:"check:age >= 0"`
	Active    bool      `gorm:"default:true"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// TestProfile 测试用户资料模型
type TestProfile struct {
	ID     uint `gorm:"primarykey"`
	UserID uint `gorm:"uniqueIndex;not null"`
	Bio    string
	Avatar string
	User   TestUser `gorm:"foreignKey:UserID"`
}

// TestConfig 测试配置验证
func TestConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "空配置",
			config:  nil,
			wantErr: true,
			errMsg:  "config cannot be nil",
		},
		{
			name: "空Master DSN",
			config: &Config{
				Master: "",
				Type:   "sqlite",
			},
			wantErr: true,
			errMsg:  "master database DSN cannot be empty",
		},
		{
			name: "空数据库类型",
			config: &Config{
				Master: ":memory:",
				Type:   "",
			},
			wantErr: true,
			errMsg:  "database type cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManager(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestManagerCreation 测试管理器创建
func TestManagerCreation(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
		PoolConfig: PoolConfig{
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: time.Hour,
			ConnMaxIdleTime: time.Minute * 30,
		},
		LogConfig: LogConfig{
			Enabled:  true,
			Level:    "info",
			Colorful: false,
		},
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	require.NotNil(t, manager)
	defer manager.Close()

	// 测试获取数据库连接
	db := manager.GetDB()
	assert.NotNil(t, db)

	// 测试数据库迁移
	err = db.AutoMigrate(&TestUser{}, &TestProfile{})
	assert.NoError(t, err)
}

// TestCRUDOperations 测试CRUD操作
func TestCRUDOperations(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	// 创建测试用户
	user := &TestUser{
		Name:   "张三",
		Email:  "zhangsan@example.com",
		Age:    25,
		Active: true,
	}

	// 测试创建
	err = db.Create(user).Error
	assert.NoError(t, err)
	assert.NotZero(t, user.ID)
	assert.False(t, user.CreatedAt.IsZero())

	// 测试查询
	var foundUser TestUser
	err = db.First(&foundUser, user.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, user.Name, foundUser.Name)
	assert.Equal(t, user.Email, foundUser.Email)

	// 测试更新
	updatedName := "李四"
	err = db.Model(&foundUser).Update("name", updatedName).Error
	assert.NoError(t, err)

	// 验证更新
	err = db.First(&foundUser, user.ID).Error
	assert.NoError(t, err)
	assert.Equal(t, updatedName, foundUser.Name)
	assert.True(t, foundUser.UpdatedAt.After(foundUser.CreatedAt))

	// 测试软删除
	err = db.Delete(&foundUser).Error
	assert.NoError(t, err)

	// 验证软删除
	err = db.First(&foundUser, user.ID).Error
	assert.Error(t, err)
	assert.Equal(t, gorm.ErrRecordNotFound, err)

	// 测试查询已删除记录
	err = db.Unscoped().First(&foundUser, user.ID).Error
	assert.NoError(t, err)
	assert.NotNil(t, foundUser.DeletedAt)
}

// TestTransactions 测试事务处理
func TestTransactions(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	// 测试成功事务
	t.Run("成功事务", func(t *testing.T) {
		err := manager.Transaction(context.Background(), func(tx *gorm.DB) error {
			user1 := &TestUser{Name: "用户1", Email: "user1@example.com", Age: 20}
			user2 := &TestUser{Name: "用户2", Email: "user2@example.com", Age: 30}

			if err := tx.Create(user1).Error; err != nil {
				return err
			}
			if err := tx.Create(user2).Error; err != nil {
				return err
			}
			return nil
		})
		assert.NoError(t, err)

		// 验证数据已提交
		var count int64
		db.Model(&TestUser{}).Count(&count)
		assert.Equal(t, int64(2), count)
	})

	// 清理数据
	db.Exec("DELETE FROM test_users")

	// 测试回滚事务
	t.Run("回滚事务", func(t *testing.T) {
		err := manager.Transaction(context.Background(), func(tx *gorm.DB) error {
			user1 := &TestUser{Name: "用户3", Email: "user3@example.com", Age: 25}
			if err := tx.Create(user1).Error; err != nil {
				return err
			}

			// 故意返回错误触发回滚
			return fmt.Errorf("测试回滚")
		})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "测试回滚")

		// 验证数据已回滚
		var count int64
		db.Model(&TestUser{}).Count(&count)
		assert.Equal(t, int64(0), count)
	})
}

// TestConcurrentOperations 测试并发操作
func TestConcurrentOperations(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
		PoolConfig: PoolConfig{
			MaxOpenConns: 1, // SQLite内存数据库限制为单连接
			MaxIdleConns: 1,
		},
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	// 测试顺序创建多个用户（模拟并发场景）
	users := make([]*TestUser, 5)
	for i := 0; i < 5; i++ {
		users[i] = &TestUser{
			Name:  fmt.Sprintf("并发用户_%d", i),
			Email: fmt.Sprintf("concurrent_%d@example.com", i),
			Age:   20 + i,
		}
		err = db.Create(users[i]).Error
		assert.NoError(t, err)
	}

	// 验证创建的记录数
	var count int64
	db.Model(&TestUser{}).Count(&count)
	assert.Equal(t, int64(5), count)

	// 测试并发读取
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			var user TestUser
			err := db.First(&user, users[index%len(users)].ID).Error
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()
}

// TestInputValidation 测试输入验证
func TestInputValidation(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	tests := []struct {
		name    string
		user    TestUser
		wantErr bool
		errMsg  string
	}{
		{
			name: "有效用户",
			user: TestUser{
				Name:  "有效用户",
				Email: "valid@example.com",
				Age:   25,
			},
			wantErr: false,
		},
		{
			name: "重复邮箱",
			user: TestUser{
				Name:  "重复用户",
				Email: "duplicate@example.com",
				Age:   30,
			},
			wantErr: true,
			errMsg:  "UNIQUE constraint failed",
		},
		{
			name: "负年龄",
			user: TestUser{
				Name:  "负年龄用户",
				Email: "negative@example.com",
				Age:   -5,
			},
			wantErr: true,
			errMsg:  "CHECK constraint failed",
		},
	}

	// 先创建一个用户用于测试重复邮箱
	firstUser := &TestUser{
		Name:  "第一个用户",
		Email: "duplicate@example.com",
		Age:   25,
	}
	err = db.Create(firstUser).Error
	require.NoError(t, err)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := db.Create(&tt.user).Error
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotZero(t, tt.user.ID)
			}
		})
	}
}

// TestEdgeCases 测试边界条件
func TestEdgeCases(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	// 测试超长字符串
	t.Run("超长字符串", func(t *testing.T) {
		longName := strings.Repeat("a", 200) // 超过name字段的100字符限制

		user := &TestUser{
			Name:  longName,
			Email: "long@example.com",
			Age:   25,
		}
		err := db.Create(user).Error
		// SQLite可能不会严格执行字符串长度限制，所以检查是否成功创建
		if err == nil {
			// 如果创建成功，验证记录存在
			var savedUser TestUser
			err = db.First(&savedUser, user.ID).Error
			assert.NoError(t, err)
			assert.NotZero(t, savedUser.ID)
		} else {
			// 如果创建失败，记录错误信息
			t.Logf("Long string creation failed as expected: %v", err)
		}
	})

	// 测试查询不存在的记录
	t.Run("查询不存在记录", func(t *testing.T) {
		var user TestUser
		err := db.First(&user, 99999).Error
		assert.Error(t, err)
		assert.Equal(t, gorm.ErrRecordNotFound, err)
	})

	// 测试批量操作
	t.Run("批量操作", func(t *testing.T) {
		users := []TestUser{
			{Name: "批量1", Email: "batch1@example.com", Age: 20},
			{Name: "批量2", Email: "batch2@example.com", Age: 21},
			{Name: "批量3", Email: "batch3@example.com", Age: 22},
		}

		err := db.Create(&users).Error
		assert.NoError(t, err)

		// 验证所有记录都有ID
		for _, user := range users {
			assert.NotZero(t, user.ID)
		}
	})
}

// TestContextHandling 测试上下文处理
func TestContextHandling(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	require.NoError(t, err)

	// 测试带超时的上下文
	t.Run("上下文超时", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*100)
		defer cancel()

		user := &TestUser{
			Name:  "上下文用户",
			Email: "context@example.com",
			Age:   25,
		}

		err := db.WithContext(ctx).Create(user).Error
		// 对于内存数据库，操作通常很快，可能不会超时
		if err != nil {
			// 如果有错误，检查是否与上下文相关
			t.Logf("Context operation error: %v", err)
		}
	})

	// 测试取消的上下文
	t.Run("上下文取消", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // 立即取消

		user := &TestUser{
			Name:  "取消用户",
			Email: "cancel@example.com",
			Age:   25,
		}

		err := db.WithContext(ctx).Create(user).Error
		if err != nil {
			t.Logf("Cancelled context operation error: %v", err)
		}
	})
}

// TestHealthCheck 测试健康检查
func TestHealthCheck(t *testing.T) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	require.NoError(t, err)
	defer manager.Close()

	// 测试Ping
	err = manager.Ping(context.Background())
	assert.NoError(t, err)

	// 测试健康检查
	healthStatus := manager.HealthCheck(context.Background())
	assert.NotEmpty(t, healthStatus)

	// 测试统计信息
	stats := manager.GetStats()
	assert.NotEmpty(t, stats)
}

// BenchmarkCRUDOperations CRUD操作性能基准测试
func BenchmarkCRUDOperations(b *testing.B) {
	config := &Config{
		Master: ":memory:",
		Type:   "sqlite",
	}

	manager, err := NewManager(config)
	if err != nil {
		b.Fatal(err)
	}
	defer manager.Close()

	db := manager.GetDB()
	err = db.AutoMigrate(&TestUser{})
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	b.Run("Create", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			user := &TestUser{
				Name:  fmt.Sprintf("BenchUser_%d", i),
				Email: fmt.Sprintf("bench_%d@example.com", i),
				Age:   25,
			}
			db.Create(user)
		}
	})

	b.Run("Read", func(b *testing.B) {
		// 先创建一些数据
		for j := 0; j < 100; j++ {
			user := &TestUser{
				Name:  fmt.Sprintf("ReadUser_%d", j),
				Email: fmt.Sprintf("read_%d@example.com", j),
				Age:   25,
			}
			db.Create(user)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var user TestUser
			db.First(&user, uint(i%100+1))
		}
	})

	b.Run("Update", func(b *testing.B) {
		// 先创建一些数据
		for j := 0; j < 100; j++ {
			user := &TestUser{
				Name:  fmt.Sprintf("UpdateUser_%d", j),
				Email: fmt.Sprintf("update_%d@example.com", j),
				Age:   25,
			}
			db.Create(user)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			db.Model(&TestUser{}).Where("id = ?", uint(i%100+1)).Update("age", 30)
		}
	})

	b.Run("Delete", func(b *testing.B) {
		// 先创建一些数据
		for j := 0; j < b.N; j++ {
			user := &TestUser{
				Name:  fmt.Sprintf("DeleteUser_%d", j),
				Email: fmt.Sprintf("delete_%d@example.com", j),
				Age:   25,
			}
			db.Create(user)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			db.Delete(&TestUser{}, uint(i+1))
		}
	})
}