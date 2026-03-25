package web

import (
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handleMessages(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	messages, err := h.msgRepo.GetByContactID(r.Context(), id)
	if err != nil {
		log.Printf("Web: Error fetching messages for contact %s: %v", id, err)
		http.Error(w, err.Error(), 500)
		return
	}

	if len(messages) == 0 {
		w.Write([]byte(`<p class="text-gray-500 italic p-4">No messages yet</p>`))
		return
	}

	fmt.Fprintf(w, `<div class="flex flex-col space-y-2 p-2" hx-get="/messages/%s" hx-trigger="every 3s" hx-swap="outerHTML">`, id)
	for _, m := range messages {
		align := "items-start"
		bgColor := "bg-blue-100"
		if !m.IsIncoming {
			align = "items-end"
			bgColor = "bg-green-100"
		}

		displayTime := m.Timestamp
		t, err := time.Parse("2006-01-02 15:04:05", m.Timestamp)
		if err == nil {
			displayTime = t.Format("Jan 02, 15:04")
		} else {
			t, err = time.Parse(time.RFC3339, m.Timestamp)
			if err == nil {
				displayTime = t.Format("Jan 02, 15:04")
			}
		}

		fmt.Fprintf(w, `
			<div class="flex flex-col %s">
				<div class="%s p-3 rounded-lg max-w-[85%%] shadow-sm">
					<p class="text-sm text-gray-800 whitespace-pre-wrap">%s</p>
					<p class="text-[9px] text-gray-500 mt-1 text-right">%s</p>
				</div>
			</div>
		`, align, bgColor, html.EscapeString(m.Text), displayTime)
	}
	fmt.Fprint(w, `</div>`)
}
