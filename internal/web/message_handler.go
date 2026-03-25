package web

import (
	"log"
	"net/http"
	"strings"
)

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	messages, err := h.msgRepo.GetByContactID(r.Context(), id)
	if err != nil {
		log.Printf("Web: Error fetching messages for contact %s: %v", id, err)
		http.Error(w, err.Error(), 500)
		return
	}

	h.templates.RenderWithStatus(w, "fragments/message_list.html", http.StatusOK, struct {
		ContactID string
		Messages  interface{}
	}{
		ContactID: id,
		Messages:  messages,
	})
}
