package logger

import (
	"fmt"
	"io"
	"os"
	"sync"

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

var (
	// L is the global logger instance
	L *Logger

	mu                sync.RWMutex
	enabledCategories = make(map[Category]bool)
	currentLevel      = zapcore.InfoLevel
)

func init() {
	// Initialize with default categories enabled
	enabledCategories[Sync] = true
	enabledCategories[AddSequence] = true
	enabledCategories[History] = true
	enabledCategories[Telegram] = true
	enabledCategories[HH] = true
	enabledCategories[Reports] = true
	enabledCategories[Messaging] = true
	enabledCategories[Filters] = true
	enabledCategories[System] = true
}

// Logger is a thin wrapper around zap.Logger
type Logger struct {
	*zap.Logger
}

func (l *Logger) WithCat(cat Category) *zap.Logger {
	return l.Logger.With(zap.String("category", string(cat)))
}

func NewLogger(output io.Writer, db *sqlx.DB) *Logger {
	config := zap.NewDevelopmentEncoderConfig()
	config.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncodeTime = zapcore.ISO8601TimeEncoder

	// Custom level enabler that respects our dynamic settings
	levelEnabler := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		mu.RLock()
		defer mu.RUnlock()
		return l >= currentLevel
	})

	// 1. Console Core (Colorized)
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(config),
		zapcore.AddSync(os.Stdout),
		levelEnabler,
	)

	// 2. Streamer Core (JSON for UI)
	streamerCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(output),
		levelEnabler,
	)

	// Wrap cores with category filter
	cores := []zapcore.Core{
		&filteringCore{Core: consoleCore},
		&filteringCore{Core: streamerCore},
	}

	// 3. DB Core (Persistent)
	if db != nil {
		cores = append(cores, &filteringCore{
			Core: &dbCore{db: db},
		})
	}

	core := zapcore.NewTee(cores...)
	return &Logger{zap.New(core)}
}

// filteringCore wraps any core to provide category-based filtering
type filteringCore struct {
	zapcore.Core
}

func (f *filteringCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if f.Enabled(ent.Level) {
		return ce.AddCore(ent, f)
	}
	return ce
}

func (f *filteringCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	category := "system"
	for _, field := range fields {
		if field.Key == "category" {
			category = field.String
			break
		}
	}

	mu.RLock()
	enabled := enabledCategories[Category(category)] || Category(category) == System || category == "system"
	mu.RUnlock()

	// Always allow Errors+ regardless of category settings
	if !enabled && ent.Level < zapcore.ErrorLevel {
		return nil
	}

	return f.Core.Write(ent, fields)
}

func (f *filteringCore) With(fields []zapcore.Field) zapcore.Core {
	return &filteringCore{Core: f.Core.With(fields)}
}

// dbCore implements zapcore.Core to write logs to SQLite
type dbCore struct {
	db *sqlx.DB
}

func (d *dbCore) Enabled(lvl zapcore.Level) bool {
	return true
}

func (d *dbCore) With(fields []zapcore.Field) zapcore.Core {
	return d
}

func (d *dbCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return ce.AddCore(ent, d)
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
func Enable(cat Category) {
	mu.Lock()
	defer mu.Unlock()
	enabledCategories[cat] = true
}

func Disable(cat Category) {
	mu.Lock()
	defer mu.Unlock()
	enabledCategories[cat] = false
}

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	switch l {
	case 0, 1:
		currentLevel = zapcore.DebugLevel
	case 2:
		currentLevel = zapcore.InfoLevel
	case 3:
		currentLevel = zapcore.WarnLevel
	case 4:
		currentLevel = zapcore.ErrorLevel
	}
}

func GetConfig() (map[Category]bool, Level) {
	mu.RLock()
	defer mu.RUnlock()
	conf := make(map[Category]bool)
	for k, v := range enabledCategories {
		conf[k] = v
	}

	var lvl Level = 2
	switch currentLevel {
	case zapcore.DebugLevel:
		lvl = 1
	case zapcore.InfoLevel:
		lvl = 2
	case zapcore.WarnLevel:
		lvl = 3
	case zapcore.ErrorLevel:
		lvl = 4
	}
	return conf, lvl
}
