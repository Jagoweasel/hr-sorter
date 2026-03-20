package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"hr-sorter/internal/database"
	"hr-sorter/internal/models"
)

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/contacts", handleContacts)
	mux.HandleFunc("/messages/", handleMessages)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/index.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", nil)
}

func handleContacts(w http.ResponseWriter, r *http.Request) {
	var contacts []models.Contact
	err := database.DB.Select(&contacts, "SELECT * FROM contacts ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, c := range contacts {
		fmt.Fprintf(w, `
			<div class="p-2 border-b cursor-pointer hover:bg-blue-50" hx-get="/messages/%d" hx-target="#chat-history">
				<p class="font-medium text-blue-700">%s %s</p>
				<p class="text-sm text-gray-500">@%s</p>
			</div>
		`, c.ID, c.FirstName, c.LastName, c.Username)
	}
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	var messages []models.Message
	err := database.DB.Select(&messages, "SELECT * FROM messages WHERE contact_id = ? ORDER BY timestamp ASC", id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	for _, m := range messages {
		bgColor := "bg-blue-100 self-start text-left"
		if !m.IsIncoming {
			bgColor = "bg-green-100 self-end text-right"
		}
		fmt.Fprintf(w, `
			<div class="%s p-2 rounded m-1 max-w-[80%%]">
				<p class="text-sm">%s</p>
				<p class="text-[10px] text-gray-400">%s</p>
			</div>
		`, bgColor, m.Text, m.Timestamp.Format("15:04"))
	}
}
