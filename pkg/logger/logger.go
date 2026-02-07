package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new logger instance
func New() *Logger {
	// Determine log level from environment
	level := zapcore.InfoLevel
	if os.Getenv("NODE_ENV") == "development" {
		level = zapcore.DebugLevel
	}

	// Custom encoder config for pretty output
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder,
		EncodeTime:     customTimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create console encoder
	consoleEncoder := zapcore.NewConsoleEncoder(encoderConfig)

	// Create core
	core := zapcore.NewCore(
		consoleEncoder,
		zapcore.AddSync(os.Stdout),
		level,
	)

	// Create logger
	// Only add stack traces for DPanic level and above (panics), not for regular errors
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.DPanicLevel))

	return &Logger{logger.Sugar()}
}

// customTimeEncoder formats time as YYYY-MM-DD HH:mm:ss.SSS
func customTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

// WithFields returns a logger with additional fields
func (l *Logger) WithFields(fields ...interface{}) *Logger {
	return &Logger{l.SugaredLogger.With(fields...)}
}

// Named returns a named logger
func (l *Logger) Named(name string) *Logger {
	return &Logger{l.SugaredLogger.Named(name)}
}
