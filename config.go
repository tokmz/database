package database

import "time"

// Config 数据库配置结构体
// 包含主从数据库配置、连接池配置、日志配置和慢查询配置
type Config struct {
	// 主库配置
	Master string `json:"master" yaml:"master" mapstructure:"master"`
	// 从库配置
	Slaves []SlaveConfig `json:"slaves" yaml:"slaves" mapstructure:"slaves"`
	// 数据库类型 (mysql, postgres, sqlite等)
	Type string `json:"type" yaml:"type" mapstructure:"type"`
	// 连接池配置
	PoolConfig PoolConfig `json:"pool_config" yaml:"pool_config" mapstructure:"pool_config"`
	// 日志配置
	LogConfig LogConfig `json:"log_config" yaml:"log_config" mapstructure:"log_config"`
	// 慢查询配置
	SlowQueryConfig SlowQueryConfig `json:"slow_query_config" yaml:"slow_query_config" mapstructure:"slow_query_config"`
	// 监控配置
	MonitorConfig MonitorConfig `json:"monitor_config" yaml:"monitor_config" mapstructure:"monitor_config"`
}

// SlaveConfig 从库配置结构体
// 包含从库连接信息、权重和连接池配置
type SlaveConfig struct {
	DSN string `json:"dsn" yaml:"dsn" mapstructure:"dsn"`
	// 数据库类型
	Type string `json:"type" yaml:"type" mapstructure:"type"`
	// 从库权重，用于负载均衡
	Weight int `json:"weight" yaml:"weight" mapstructure:"weight"`
	// 连接池配置
	PoolConfig PoolConfig `json:"pool_config" yaml:"pool_config" mapstructure:"pool_config"`
}

// PoolConfig 连接池配置结构体
// 用于控制数据库连接池的各项参数
type PoolConfig struct {
	// 连接池最大连接数
	MaxOpenConns int `json:"max_open_conns" yaml:"max_open_conns" mapstructure:"max_open_conns"`
	// 连接池最大空闲连接数
	MaxIdleConns int `json:"max_idle_conns" yaml:"max_idle_conns" mapstructure:"max_idle_conns"`
	// 连接池最大连接生命周期
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime" yaml:"conn_max_lifetime" mapstructure:"conn_max_lifetime"`
	// 连接池最大连接空闲时间
	ConnMaxIdleTime time.Duration `json:"conn_max_idle_time" yaml:"conn_max_idle_time" mapstructure:"conn_max_idle_time"`
}

// LogConfig 日志配置结构体
// 支持多种日志级别和输出方式
type LogConfig struct {
	// 是否启用日志
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	// 日志级别 (silent, error, warn, info)
	Level string `json:"level" yaml:"level" mapstructure:"level"`
	// 是否启用彩色输出
	Colorful bool `json:"colorful" yaml:"colorful" mapstructure:"colorful"`
	// 是否忽略记录未找到的错误
	IgnoreRecordNotFoundError bool `json:"ignore_record_not_found_error" yaml:"ignore_record_not_found_error" mapstructure:"ignore_record_not_found_error"`
	// 是否记录参数化查询
	ParameterizedQueries bool `json:"parameterized_queries" yaml:"parameterized_queries" mapstructure:"parameterized_queries"`
}

// SlowQueryConfig 慢查询配置结构体
// 用于监控和记录执行时间超过阈值的SQL查询
type SlowQueryConfig struct {
	// 是否启用慢查询监控
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	// 慢查询阈值，超过此时间的查询将被记录
	Threshold time.Duration `json:"threshold" yaml:"threshold" mapstructure:"threshold"`
	// 是否记录查询参数
	LogParams bool `json:"log_params" yaml:"log_params" mapstructure:"log_params"`
}

// MonitorConfig 监控配置结构体
// 用于配置数据库连接状态监控
type MonitorConfig struct {
	// 是否启用监控
	Enabled bool `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	// 健康检查间隔
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval" mapstructure:"health_check_interval"`
	// 连接超时时间
	ConnectionTimeout time.Duration `json:"connection_timeout" yaml:"connection_timeout" mapstructure:"connection_timeout"`
	// 最大重试次数
	MaxRetries int `json:"max_retries" yaml:"max_retries" mapstructure:"max_retries"`
}
