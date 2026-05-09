package logger

import (
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	instance *zap.Logger
	once     sync.Once
)

func Init(level string) {
	once.Do(func() {
		lvl := zapcore.InfoLevel
		if err := lvl.UnmarshalText([]byte(level)); err != nil {
			lvl = zapcore.InfoLevel
		}
		enc := zapcore.NewJSONEncoder(zapcore.EncoderConfig{
			TimeKey:        "timestamp",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.StringDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		})
		core := zapcore.NewCore(enc, zapcore.AddSync(os.Stdout), lvl)
		instance = zap.New(core, zap.AddCaller())
	})
}

func Get() *zap.Logger {
	if instance == nil {
		Init("info")
	}
	return instance
}
