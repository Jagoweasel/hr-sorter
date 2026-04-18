package logger

import (
	"hr-sorter/pkg/logger"
)

type Category = logger.Category

const (
	Sync        = logger.Sync
	AddSequence = logger.AddSequence
	History     = logger.History
	Telegram    = logger.Telegram
	HH          = logger.HH
	Reports     = logger.Reports
	Messaging   = logger.Messaging
	Filters     = logger.Filters
	TraceCat    = logger.TraceCat
	HHNet       = logger.HHNet
)

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

func Enable(cat Category) {
	// No-op in new unified logger for now, or you can implement dynamic level switching
}

func Disable(cat Category) {
	// No-op
}

func SetLevel(l Level) {
	// No-op
}

func IsEnabled(cat Category) bool {
	return true // Zap handles this internally
}

func GetConfig() (map[Category]bool, Level) {
	return make(map[Category]bool), 2 // Default INFO
}

func Debug(cat Category, format string, v ...interface{}) {
	logger.Debug(cat, format, v...)
}

func Trace(cat Category, format string, v ...interface{}) {
	logger.Trace(cat, format, v...)
}

func Info(cat Category, format string, v ...interface{}) {
	logger.Info(cat, format, v...)
}

func Warn(cat Category, format string, v ...interface{}) {
	logger.Warn(cat, format, v...)
}

func Error(cat Category, format string, v ...interface{}) {
	logger.Error(cat, format, v...)
}

func LogChain(seqID int64, company string, stages []string, status string) {
	logger.LogChain(seqID, company, stages, status)
}
