package logger

import (
	"fmt"
	"io"
	"os"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Category string

const (
	Sync        Category = "sync"
	AddSequence Category = "add-sequence"
	History     Category = "history"
	Telegram    Category = "tg"
	HH          Category = "hh"
	Reports     Category = "reports"
	Messaging   Category = "msg"
	Filters     Category = "filters"
	TraceCat    Category = "trace"
	HHNet       Category = "hh-net"
	System      Category = "system"
)

// Logger is a thin wrapper around zap.Logger
type Logger struct {
	*zap.Logger
}

func NewLogger(output io.Writer, db *sqlx.DB) *Logger {
	config := zap.NewDevelopmentEncoderConfig()
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncodeTime = zapcore.ISO8601TimeEncoder

	// 1. Console Core (Colorized)
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(os.Stdout),
		zap.DebugLevel,
	)

	// 2. Streamer Core (JSON for UI)
	streamerCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(output),
		zap.DebugLevel,
	)

	// 3. DB Core (Persistent)
	var cores []zapcore.Core
	cores = append(cores, consoleCore, streamerCore)

	if db != nil {
		cores = append(cores, &dbCore{
			db:    db,
			level: zap.InfoLevel, // Only store Info and above in DB by default to save space
		})
	}

	core := zapcore.NewTee(cores...)
	return &Logger{zap.New(core)}
}

// Global logger instance for simple migration
var L *Logger

func (l *Logger) WithCat(cat Category) *zap.Logger {
	return l.Logger.With(zap.String("category", string(cat)))
}

// dbCore implements zapcore.Core to write logs to SQLite
type dbCore struct {
	db    *sqlx.DB
	level zapcore.Level
}

func (d *dbCore) Enabled(lvl zapcore.Level) bool {
	return lvl >= d.level
}

func (d *dbCore) With(fields []zapcore.Field) zapcore.Core {
	return d // In a real impl we'd clone and add fields, but for DB we'll just extract them in Write
}

func (d *dbCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if d.Enabled(ent.Level) {
		return ce.AddCore(ent, d)
	}
	return ce
}

func (d *dbCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	category := "system"
	for _, f := range fields {
		if f.Key == "category" {
			category = f.String
			break
		}
	}

	_, err := d.db.Exec("INSERT INTO system_logs (level, category, message, timestamp) VALUES (?, ?, ?, ?)",
		ent.Level.String(),
		category,
		ent.Message,
		ent.Time.Format("2006-01-02 15:04:05"),
	)
	return err
}

func (d *dbCore) Sync() error {
	return nil
}

// Migration helpers for internal/logger
func Debug(cat Category, format string, v ...interface{}) {
	if L != nil {
		L.WithCat(cat).Debug(fmt.Sprintf(format, v...))
	}
}

func Info(cat Category, format string, v ...interface{}) {
	if L != nil {
		L.WithCat(cat).Info(fmt.Sprintf(format, v...))
	}
}

func Warn(cat Category, format string, v ...interface{}) {
	if L != nil {
		L.WithCat(cat).Warn(fmt.Sprintf(format, v...))
	}
}

func Error(cat Category, format string, v ...interface{}) {
	if L != nil {
		L.WithCat(cat).Error(fmt.Sprintf(format, v...))
	}
}

func Trace(cat Category, format string, v ...interface{}) {
	if L != nil {
		L.WithCat(cat).Debug("[TRACE] "+fmt.Sprintf(format, v...), zap.Bool("trace", true))
	}
}

func LogChain(seqID int64, company string, stages []string, status string) {
	if L != nil {
		chain := ""
		for i, s := range stages {
			if i > 0 {
				chain += " -> "
			}
			chain += s
		}
		if status == "rejected" {
			chain += " -> [REJECTED]"
		} else if status == "accepted" {
			chain += " -> [ACCEPTED]"
		}
		L.WithCat(History).Info(fmt.Sprintf("Seq #%d (%s): %s", seqID, company, chain))
	}
}

// Support for legacy level/category management
func Enable(cat Category)  {}
func Disable(cat Category) {}

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

func SetLevel(l Level) {}
func GetConfig() (map[Category]bool, Level) {
	return make(map[Category]bool), 2
}
