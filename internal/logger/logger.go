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
)

var enabledCategories = make(map[Category]bool)

func Enable(cat Category) {
	enabledCategories[cat] = true
}

func IsEnabled(cat Category) bool {
	return enabledCategories[cat]
}

func Debug(cat Category, format string, v ...interface{}) {
	if enabledCategories[cat] {
		log.Printf("[%s] %s", strings.ToUpper(string(cat)), fmt.Sprintf(format, v...))
	}
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
