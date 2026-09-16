// Package logger 提供基于 zap 的结构化日志构造。
//
// 与业务无关，仅依赖 level/format/output 三个原语参数，便于独立复用与测试。
package logger

import (
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 支持的日志级别 / 输出目标常量。
const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	FormatConsole = "console"
	FormatJSON    = "json"

	OutputStdout = "stdout"
	OutputStderr = "stderr"
)

// New 构造一个 zap.Logger。
//
//   - level:  debug | info | warn | error（大小写不敏感，默认 info）
//   - format: console | json（默认 console）
//   - output: stdout | stderr | 文件路径（默认 stdout）
func New(level, format, output string) (*zap.Logger, error) {
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}

	encoder := newEncoder(format)

	ws, err := newWriteSyncer(output)
	if err != nil {
		return nil, err
	}

	core := zapcore.NewCore(encoder, ws, lvl)
	return zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel)), nil
}

func parseLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "", LevelInfo:
		return zapcore.InfoLevel, nil
	case LevelDebug:
		return zapcore.DebugLevel, nil
	case LevelWarn, "warning":
		return zapcore.WarnLevel, nil
	case LevelError:
		return zapcore.ErrorLevel, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("非法的日志级别 %q（可选 debug|info|warn|error）", level)
	}
}

func newEncoder(format string) zapcore.Encoder {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encCfg.EncodeDuration = zapcore.StringDurationEncoder

	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatJSON:
		return zapcore.NewJSONEncoder(encCfg)
	default:
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		return zapcore.NewConsoleEncoder(encCfg)
	}
}

func newWriteSyncer(output string) (zapcore.WriteSyncer, error) {
	switch strings.ToLower(strings.TrimSpace(output)) {
	case "", OutputStdout:
		return zapcore.AddSync(os.Stdout), nil
	case OutputStderr:
		return zapcore.AddSync(os.Stderr), nil
	default:
		f, err := os.OpenFile(output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("打开日志文件 %q 失败: %w", output, err)
		}
		return zapcore.AddSync(f), nil
	}
}
