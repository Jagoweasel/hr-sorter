package web

import (
	"log"
	"net/http"
)

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	h.templates.RenderWithStatus(w, "logs.html", http.StatusOK, nil, h.getLocale(r))
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
