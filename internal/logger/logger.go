package logger

import (
	"fmt"
	"log"
	"strings"
	"sync"
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
)

type Level int

const (
	LevelTrace Level = iota
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
)

var (
	enabledCategories = make(map[Category]bool)
	mu                sync.RWMutex
	minLevel          = LevelInfo
)

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

func SetLevel(l Level) {
	mu.Lock()
	defer mu.Unlock()
	minLevel = l
}

func IsEnabled(cat Category) bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabledCategories[cat] || enabledCategories[TraceCat]
}

func GetConfig() (map[Category]bool, Level) {
	mu.RLock()
	defer mu.RUnlock()
	conf := make(map[Category]bool)
	for k, v := range enabledCategories {
		conf[k] = v
	}
	return conf, minLevel
}

func Debug(cat Category, format string, v ...interface{}) {
	mu.RLock()
	enabled := enabledCategories[cat] || enabledCategories[TraceCat]
	lvl := minLevel
	mu.RUnlock()

	if (enabled || lvl <= LevelDebug) && lvl <= LevelDebug {
		log.Printf("[DEBUG][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Trace(cat Category, format string, v ...interface{}) {
	mu.RLock()
	enabled := enabledCategories[TraceCat]
	lvl := minLevel
	mu.RUnlock()

	if (enabled || lvl <= LevelTrace) && lvl <= LevelTrace {
		log.Printf("[TRACE][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Info(cat Category, format string, v ...interface{}) {
	mu.RLock()
	lvl := minLevel
	mu.RUnlock()

	if lvl <= LevelInfo {
		log.Printf("[INFO][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Warn(cat Category, format string, v ...interface{}) {
	mu.RLock()
	lvl := minLevel
	mu.RUnlock()

	if lvl <= LevelWarn {
		log.Printf("[WARN][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Error(cat Category, format string, v ...interface{}) {
	mu.RLock()
	lvl := minLevel
	mu.RUnlock()

	if lvl <= LevelError {
		log.Printf("[ERROR][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

// LogChain outputs a visual representation of the sequence history
func LogChain(seqID int64, company string, stages []string, status string) {
	mu.RLock()
	enabled := enabledCategories[History]
	mu.RUnlock()

	if !enabled {
		return
	}
	chain := strings.Join(stages, " -> ")
	if status == "rejected" {
		chain += " -> [REJECTED]"
	} else if status == "accepted" {
		chain += " -> [ACCEPTED]"
	}
	log.Printf("[HISTORY] Seq #%d (%s): %s", seqID, company, chain)
}
