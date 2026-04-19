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
	logger.Enable(cat)
}

func Disable(cat Category) {
	logger.Disable(cat)
}

func SetLevel(l Level) {
	logger.SetLevel(logger.Level(l))
}

func IsEnabled(cat Category) bool {
	conf, _ := logger.GetConfig()
	return conf[cat]
}

func GetConfig() (map[Category]bool, Level) {
	conf, lvl := logger.GetConfig()
	return conf, Level(lvl)
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
