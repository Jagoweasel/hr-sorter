package logger

import (
	"fmt"
	"log"
	"strings"
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

var enabledCategories = make(map[Category]bool)

func Enable(cat Category) {
	enabledCategories[cat] = true
}

func IsEnabled(cat Category) bool {
	return enabledCategories[cat]
}

func Debug(cat Category, format string, v ...interface{}) {
	if enabledCategories[cat] || enabledCategories[TraceCat] {
		log.Printf("[DEBUG][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Trace(cat Category, format string, v ...interface{}) {
	if enabledCategories[TraceCat] {
		log.Printf("[TRACE][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
}

func Info(cat Category, format string, v ...interface{}) {
	log.Printf("[INFO][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
}

func Error(cat Category, format string, v ...interface{}) {
	log.Printf("[ERROR][%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
}

// LogChain outputs a visual representation of the sequence history
func LogChain(seqID int64, company string, stages []string, status string) {
	if !enabledCategories[History] {
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
