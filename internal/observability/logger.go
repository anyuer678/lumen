package observability

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var globalLogger *zap.Logger

func Init(logFile string, level string) (*zap.Logger, error) {
	// 解析日志级别
	var logLevel zapcore.Level
	switch level {
	case "debug":
		logLevel = zapcore.DebugLevel
	case "warn":
		logLevel = zapcore.WarnLevel
	case "error":
		logLevel = zapcore.ErrorLevel
	default:
		logLevel = zapcore.InfoLevel
	}

	// 编码器配置
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "ts"
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// 控制台输出
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		logLevel,
	)

	cores := []zapcore.Core{consoleCore}

	// 文件输出
	if logFile != "" {
		dir := filepath.Dir(logFile)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}

		f, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}

		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderConfig),
			zapcore.AddSync(f),
			logLevel,
		)
		cores = append(cores, fileCore)
	}

	logger := zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	globalLogger = logger
	return logger, nil
}

func Get() *zap.Logger {
	return globalLogger
}

func Sugar() *zap.SugaredLogger {
	if globalLogger == nil {
		return zap.NewNop().Sugar()
	}
	return globalLogger.Sugar()
}

func Sync() error {
	if globalLogger != nil {
		return globalLogger.Sync()
	}
	return nil
}
