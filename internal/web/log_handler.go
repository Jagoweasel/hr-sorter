package web

import (
	"encoding/json"
	"hr-sorter/internal/logger"
	"log"
	"net/http"
	"strconv"
)

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	cats, lvl := logger.GetConfig()
	data := map[string]interface{}{
		"Categories": cats,
		"Level":      int(lvl),
	}
	h.templates.RenderWithStatus(w, r, "logs.html", http.StatusOK, data, h.getLocale(r))
}

func (h *Handler) handleUpdateLogConfig(w http.ResponseWriter, r *http.Request) {
	cat := r.URL.Query().Get("category")
	enabled := r.URL.Query().Get("enabled") == "true"
	levelStr := r.URL.Query().Get("level")

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

func (h *Handler) handleLogStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := h.logBroadcaster.Stream(r.Context(), w); err != nil {
		log.Printf("SSE error: %v", err)
	}
}
