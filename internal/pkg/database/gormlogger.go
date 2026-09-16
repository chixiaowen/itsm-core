package database

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// defaultSlowThreshold 慢查询阈值（§9.3：200ms）。
const defaultSlowThreshold = 200 * time.Millisecond

// gormZapLogger 是 GORM logger.Interface 的 zap 适配器。
type gormZapLogger struct {
	z             *zap.Logger
	slowThreshold time.Duration
	level         logger.LogLevel
}

// newGormZapLogger 构造 GORM 日志适配器；log 为 nil 时使用 no-op logger。
func newGormZapLogger(z *zap.Logger, slowThreshold time.Duration) logger.Interface {
	if z == nil {
		z = zap.NewNop()
	}
	if slowThreshold <= 0 {
		slowThreshold = defaultSlowThreshold
	}
	return &gormZapLogger{z: z, slowThreshold: slowThreshold, level: logger.Warn}
}

// LogMode 返回调整了日志级别的新实例。
func (l *gormZapLogger) LogMode(level logger.LogLevel) logger.Interface {
	cp := *l
	cp.level = level
	return &cp
}

// Info 记录 info 级日志。
func (l *gormZapLogger) Info(_ context.Context, msg string, data ...interface{}) {
	if l.level < logger.Info {
		return
	}
	l.z.Sugar().Infof(msg, data...)
}

// Warn 记录 warn 级日志。
func (l *gormZapLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	if l.level < logger.Warn {
		return
	}
	l.z.Sugar().Warnf(msg, data...)
}

// Error 记录 error 级日志。
func (l *gormZapLogger) Error(_ context.Context, msg string, data ...interface{}) {
	if l.level < logger.Error {
		return
	}
	l.z.Sugar().Errorf(msg, data...)
}

// Trace 记录单条 SQL：错误 -> error；慢查询 -> warn；其余 -> debug。
func (l *gormZapLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()

	fields := []zap.Field{
		zap.String("sql", sql),
		zap.Int64("rows", rows),
		zap.Duration("elapsed", elapsed),
	}

	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		l.z.Error("gorm 查询错误", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold:
		l.z.Warn("gorm 慢查询", fields...)
	case l.level >= logger.Info:
		l.z.Debug("gorm 查询", fields...)
	}
}
