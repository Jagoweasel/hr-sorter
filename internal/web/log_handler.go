package web

import (
	"encoding/json"
	"hr-sorter/pkg/logger"
	"log"
	"net/http"
	"strconv"
)

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	// For now, we still use the old logger configuration to keep the UI controls working
	// But in a full migration, we should move this to Zap's AtomicLevel
	// Since we are doing a phased transition, let's just show default values or adapt.
	data := map[string]interface{}{
		"Categories": make(map[string]bool), // TODO: Link with Zap level enabler
		"Level":      2,                     // Default to INFO (logger.LevelInfo)
	}
	h.templates.RenderWithStatus(w, r, "logs.html", http.StatusOK, data, h.getLocale(r))
}

func (h *Handler) handleUpdateLogConfig(w http.ResponseWriter, r *http.Request) {
	// Support both Query (for simple fetch) and Form (for standard POST)
	cat := r.URL.Query().Get("category")
	if cat == "" {
		cat = r.FormValue("category")
	}

	enabledStr := r.URL.Query().Get("enabled")
	if enabledStr == "" {
		enabledStr = r.FormValue("enabled")
	}
	enabled := enabledStr == "true"

	levelStr := r.URL.Query().Get("level")
	if levelStr == "" {
		levelStr = r.FormValue("level")
	}

	if cat != "" {
		if enabled {
			logger.Enable(logger.Category(cat))
		} else {
			logger.Disable(logger.Category(cat))
		}
	}

	if levelStr != "" {
		if lvl, err := strconv.Atoi(levelStr); err == nil {
			logger.SetLevel(logger.Level(lvl))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	cats, lvl := logger.GetConfig()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"categories": cats,
		"level":      int(lvl),
	})
}

func (h *Handler) handleLogHistory(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}

	query := "SELECT level, category, message, timestamp FROM system_logs"
	var args []interface{}
	if category != "" && category != "ALL" {
		query += " WHERE category = ?"
		args = append(args, category)
	}
	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	var logs []struct {
		Level     string `json:"level"`
		Category  string `json:"category"`
		Message   string `json:"msg"`
		Timestamp string `json:"ts"`
	}

	err := h.db.Select(&logs, query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Reverse to show oldest first in UI
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (h *Handler) handleLogStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := h.logBroadcaster.Stream(r.Context(), w); err != nil {
		log.Printf("SSE error: %v", err)
	}
}
