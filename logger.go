package database

import (
	"context"
	"fmt"
	"log"
	"time"
)

// DefaultLogger 默认日志记录器实现
// 实现Logger接口，提供基本的日志功能
type DefaultLogger struct {
	// config 日志配置
	config LogConfig
	// logger 标准库日志记录器
	logger *log.Logger
	// logLevel 当前日志级别
	logLevel LogLevel
}

// LogMode 设置日志模式
// 参数:
//   - level: 日志级别
// 返回值:
//   - Logger: 日志记录器接口
func (l *DefaultLogger) LogMode(level LogLevel) Logger {
	newLogger := *l
	newLogger.logLevel = level
	return &newLogger
}

// Info 记录信息级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (l *DefaultLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= Info {
		l.printf("[INFO] "+msg, data...)
	}
}

// Warn 记录警告级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (l *DefaultLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= Warn {
		l.printf("[WARN] "+msg, data...)
	}
}

// Error 记录错误级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (l *DefaultLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.logLevel >= Error {
		l.printf("[ERROR] "+msg, data...)
	}
}

// Trace 记录SQL执行轨迹
// 参数:
//   - ctx: 上下文
//   - begin: 开始时间
//   - fc: 获取SQL和影响行数的函数
//   - err: 执行错误
func (l *DefaultLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.logLevel <= Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	switch {
	case err != nil && l.logLevel >= Error:
		l.printf("[ERROR] SQL执行失败 [%v] [rows:%d] %s | %v", elapsed, rows, sql, err)
	case elapsed > l.getSlowThreshold() && l.logLevel >= Warn:
		l.printf("[WARN] 慢查询检测 [%v] [rows:%d] %s", elapsed, rows, sql)
	case l.logLevel == Info:
		l.printf("[INFO] SQL执行 [%v] [rows:%d] %s", elapsed, rows, sql)
	}
}

// printf 格式化输出日志
// 参数:
//   - format: 格式字符串
//   - args: 参数列表
func (l *DefaultLogger) printf(format string, args ...interface{}) {
	l.logger.Printf(format, args...)
}

// getSlowThreshold 获取慢查询阈值
// 返回值:
//   - time.Duration: 慢查询阈值
func (l *DefaultLogger) getSlowThreshold() time.Duration {
	// 默认200ms
	return 200 * time.Millisecond
}

// SlowQueryLogger 慢查询日志记录器
// 专门用于记录和分析慢查询
type SlowQueryLogger struct {
	// config 慢查询配置
	config SlowQueryConfig
	// baseLogger 基础日志记录器
	baseLogger Logger
	// logger 标准库日志记录器
	logger *log.Logger
}

// LogMode 设置日志模式
// 参数:
//   - level: 日志级别
// 返回值:
//   - Logger: 日志记录器接口
func (s *SlowQueryLogger) LogMode(level LogLevel) Logger {
	return s
}

// Info 记录信息级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (s *SlowQueryLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if s.baseLogger != nil {
		s.baseLogger.Info(ctx, msg, data...)
	}
}

// Warn 记录警告级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (s *SlowQueryLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if s.baseLogger != nil {
		s.baseLogger.Warn(ctx, msg, data...)
	}
}

// Error 记录错误级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (s *SlowQueryLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if s.baseLogger != nil {
		s.baseLogger.Error(ctx, msg, data...)
	}
}

// Trace 记录SQL执行轨迹，重点关注慢查询
// 参数:
//   - ctx: 上下文
//   - begin: 开始时间
//   - fc: 获取SQL和影响行数的函数
//   - err: 执行错误
func (s *SlowQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if !s.config.Enabled {
		return
	}

	elapsed := time.Since(begin)

	// 只记录超过阈值的查询
	if elapsed >= s.config.Threshold {
		sql, rows := fc()
		
		// 构建慢查询日志信息
		logInfo := fmt.Sprintf("慢查询检测 - 执行时间: %v, 影响行数: %d", elapsed, rows)
		
		if s.config.LogParams {
			logInfo += fmt.Sprintf(", SQL: %s", sql)
		}
		
		if err != nil {
			logInfo += fmt.Sprintf(", 错误: %v", err)
		}
		
		s.logger.Printf("[SLOW_QUERY] %s", logInfo)
		
		// 同时通过基础日志记录器记录
		if s.baseLogger != nil {
			s.baseLogger.Warn(ctx, "检测到慢查询", "duration", elapsed, "sql", sql, "rows", rows)
		}
	}
}

// ZapLogger Zap日志记录器适配器
// 用于适配zap日志库
type ZapLogger struct {
	// zapLogger zap日志记录器实例
	// 这里使用interface{}避免强依赖zap
	zapLogger interface{}
	// logLevel 日志级别
	logLevel LogLevel
}

// NewZapLogger 创建Zap日志记录器适配器
// 参数:
//   - zapLogger: zap日志记录器实例
// 返回值:
//   - Logger: 日志记录器接口
func NewZapLogger(zapLogger interface{}) Logger {
	return &ZapLogger{
		zapLogger: zapLogger,
		logLevel:  Info,
	}
}

// LogMode 设置日志模式
// 参数:
//   - level: 日志级别
// 返回值:
//   - Logger: 日志记录器接口
func (z *ZapLogger) LogMode(level LogLevel) Logger {
	newLogger := *z
	newLogger.logLevel = level
	return &newLogger
}

// Info 记录信息级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (z *ZapLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if z.logLevel >= Info {
		// 这里应该调用zap的Info方法
		// 为了避免强依赖，这里使用反射或类型断言
		fmt.Printf("[ZAP-INFO] %s %v\n", msg, data)
	}
}

// Warn 记录警告级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (z *ZapLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if z.logLevel >= Warn {
		// 这里应该调用zap的Warn方法
		fmt.Printf("[ZAP-WARN] %s %v\n", msg, data)
	}
}

// Error 记录错误级别日志
// 参数:
//   - ctx: 上下文
//   - msg: 日志消息
//   - data: 附加数据
func (z *ZapLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if z.logLevel >= Error {
		// 这里应该调用zap的Error方法
		fmt.Printf("[ZAP-ERROR] %s %v\n", msg, data)
	}
}

// Trace 记录SQL执行轨迹
// 参数:
//   - ctx: 上下文
//   - begin: 开始时间
//   - fc: 获取SQL和影响行数的函数
//   - err: 执行错误
func (z *ZapLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if z.logLevel <= Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// 使用zap记录结构化日志
	switch {
	case err != nil && z.logLevel >= Error:
		fmt.Printf("[ZAP-ERROR] SQL执行失败: duration=%v, rows=%d, sql=%s, error=%v\n", elapsed, rows, sql, err)
	case elapsed > 200*time.Millisecond && z.logLevel >= Warn:
		fmt.Printf("[ZAP-WARN] 慢查询检测: duration=%v, rows=%d, sql=%s\n", elapsed, rows, sql)
	case z.logLevel == Info:
		fmt.Printf("[ZAP-INFO] SQL执行: duration=%v, rows=%d, sql=%s\n", elapsed, rows, sql)
	}
}