package web

import (
	"hr-sorter/internal/models"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/messages/")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	messages, err := h.msgRepo.GetByContactID(r.Context(), id)
	if err != nil {
		log.Printf("Web: Error fetching messages for contact %d: %v", id, err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Merge with HH cache if applicable
	cached := h.hhManager.GetCachedMessages(id)
	if len(cached) > 0 {
		// Use a map to de-duplicate by ExternalID
		msgMap := make(map[string]models.Message)
		for _, m := range messages {
			if m.ExternalID != nil {
				msgMap[*m.ExternalID] = m
			}
		}
		for _, m := range cached {
			if m.ExternalID != nil {
				if _, exists := msgMap[*m.ExternalID]; !exists {
					messages = append(messages, m)
				}
			}
		}
		// Sort by timestamp
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Timestamp < messages[j].Timestamp
		})
	}

	contact, _ := h.conRepo.GetByID(r.Context(), id)

	h.templates.RenderWithStatus(w, r, "fragments/chat_container.html", http.StatusOK, struct {
		ContactID int64
		Contact   interface{}
		Messages  interface{}
	}{
		ContactID: id,
		Contact:   contact,
		Messages:  messages,
	}, h.getLocale(r))
}

func (h *Handler) handleMessageList(w http.ResponseWriter, r *http.Request) {
	// Path is /messages/list/{id}
	idStr := strings.TrimPrefix(r.URL.Path, "/messages/list/")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	messages, err := h.msgRepo.GetByContactID(r.Context(), id)
	if err != nil {
		log.Printf("Web: Error fetching messages for contact %d: %v", id, err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Merge with HH cache
	cached := h.hhManager.GetCachedMessages(id)
	if len(cached) > 0 {
		msgMap := make(map[string]models.Message)
		for _, m := range messages {
			if m.ExternalID != nil {
				msgMap[*m.ExternalID] = m
			}
		}
		for _, m := range cached {
			if m.ExternalID != nil {
				if _, exists := msgMap[*m.ExternalID]; !exists {
					messages = append(messages, m)
				}
			}
		}
		// Sort by timestamp
		sort.Slice(messages, func(i, j int) bool {
			return messages[i].Timestamp < messages[j].Timestamp
		})
	}

	h.templates.RenderWithStatus(w, r, "fragments/message_list.html", http.StatusOK, struct {
		ContactID int64
		Messages  interface{}
	}{
		ContactID: id,
		Messages:  messages,
	}, h.getLocale(r))
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
