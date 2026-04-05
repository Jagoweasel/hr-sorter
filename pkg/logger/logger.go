package logger

import (
	"io"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a thin wrapper around zap.Logger
type Logger struct {
	*zap.SugaredLogger
}

func NewLogger(output io.Writer) *Logger {
	config := zap.NewDevelopmentEncoderConfig()
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder

	// Create cores: one for stdout, one for the broadcaster
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(os.Stdout),
		zap.DebugLevel,
	)

	// Streamer core uses plain JSON for easier parsing in the UI or plain text
	streamerCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(output),
		zap.DebugLevel,
	)

	core := zapcore.NewTee(consoleCore, streamerCore)
	return &Logger{zap.New(core).Sugar()}
}

func (l *Logger) Info(msg string, args ...interface{}) {
	l.SugaredLogger.Infof(msg, args...)
}

func (l *Logger) Error(msg string, err error, args ...interface{}) {
	l.SugaredLogger.Errorf(msg+": %v", append(args, err)...)
}
