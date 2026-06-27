// Package logger provides structured, leveled logging built on Zap with a
// single construction path shared by every executable. Development uses a
// human-friendly console encoder; staging/production use JSON.
package logger

import (
	"fmt"

	"github.com/agnivo/agnivo/packages/platform/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New constructs the root logger from configuration. The returned logger is
// tagged with the service name and environment so every line is attributable.
func New(cfg *config.Config) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Log.Level)
	if err != nil {
		return nil, fmt.Errorf("logger: parse level %q: %w", cfg.Log.Level, err)
	}

	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	var encoder zapcore.Encoder
	switch cfg.Log.Format {
	case "console":
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	default:
		encoder = zapcore.NewJSONEncoder(encCfg)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(stdout())), level)

	opts := []zap.Option{
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	}
	if cfg.App.Environment.IsDevelopment() {
		opts = append(opts, zap.Development())
	}

	log := zap.New(core, opts...).With(
		zap.String("service", cfg.App.Name),
		zap.String("env", string(cfg.App.Environment)),
	)
	return log, nil
}

// NewNop returns a no-op logger, useful in tests.
func NewNop() *zap.Logger { return zap.NewNop() }
