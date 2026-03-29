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

	h.templates.RenderWithStatus(w, "fragments/chat_container.html", http.StatusOK, struct {
		ContactID string
		Messages  interface{}
	}{
		ContactID: id,
		Messages:  messages,
	})
}

func (h *Handler) handleMessageList(w http.ResponseWriter, r *http.Request) {
	// Path is /messages/list/{id}
	id := strings.TrimPrefix(r.URL.Path, "/messages/list/")
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

func (h *Handler) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	// Path is /messages/{id}/send
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	id = strings.TrimSuffix(id, "/send")

	text := r.FormValue("text")
	if text == "" {
		w.WriteHeader(http.StatusOK)
		return
	}

	log.Printf("Web: Sending message to contact %s: %s", id, text)
	err := h.conService.SendChatMessage(r.Context(), id, text)
	if err != nil {
		log.Printf("Web: Error sending message: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Trigger a message list refresh on the frontend
	w.Header().Set("HX-Trigger", "refreshMessages")
	w.WriteHeader(http.StatusOK)
}
