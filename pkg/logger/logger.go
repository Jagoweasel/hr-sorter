package logger

import (
	"io"
)

// Logger is a thin wrapper around zap.Logger
type Logger struct {
	// *zap.SugaredLogger
}

func NewLogger(output io.Writer) *Logger {
	// 1. Initialize zap with Tee output (Stdout + output)
	// 2. output will be our LogBroadcaster
	panic("implement me with zap and streamer output")
}

func (l *Logger) Info(msg string, args ...interface{}) {
	panic("implement me")
}

func (l *Logger) Error(msg string, err error, args ...interface{}) {
	panic("implement me")
}
