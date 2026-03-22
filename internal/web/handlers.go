package web

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"hr-sorter/internal/database"
	"hr-sorter/internal/models"
)

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/contacts", handleContacts)
	mux.HandleFunc("/messages/", handleMessages)
	mux.HandleFunc("/accounts", handleAccounts)
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

func handleAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []models.Account
	err := database.DB.Select(&accounts, "SELECT * FROM accounts ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/accounts.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", accounts)
}

func handleContacts(w http.ResponseWriter, r *http.Request) {
	type ContactWithLastMsg struct {
		models.Contact
		LastMessage string `db:"last_message"`
		LastTime    string `db:"last_time"`
	}
	var contacts []ContactWithLastMsg
	query := `
		SELECT c.*, 
		       COALESCE((SELECT text FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), 'No messages yet') as last_message,
			   COALESCE((SELECT datetime(timestamp) FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), datetime(c.created_at)) as last_time
		FROM contacts c
		ORDER BY last_time DESC`

	err := database.DB.Select(&contacts, query)
	if err != nil {
		log.Printf("Web: Error fetching contacts: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	log.Printf("Web: Rendering %d contacts", len(contacts))

	// Simple check for new messages to trigger browser notification
	if len(contacts) > 0 {
		lt, _ := time.Parse("2006-01-02 15:04:05", contacts[0].LastTime)
		if time.Since(lt).Seconds() < 15 {
			// Only trigger if it's an incoming message
			var isIncoming bool
			database.DB.Get(&isIncoming, "SELECT is_incoming FROM messages WHERE contact_id = ? ORDER BY timestamp DESC LIMIT 1", contacts[0].ID)

			if isIncoming {
				w.Header().Set("HX-Trigger", fmt.Sprintf(`{"new-message": {"sender": "%s", "text": "%s"}}`,
					contacts[0].FirstName, strings.ReplaceAll(contacts[0].LastMessage, `"`, `'`)))
			}
		}
	}

	for _, c := range contacts {
		lastMsg := c.LastMessage
		if len(lastMsg) > 30 {
			lastMsg = lastMsg[:27] + "..."
		}
		fmt.Fprintf(w, `
			<div class="p-3 border-b cursor-pointer hover:bg-blue-50 transition-colors contact-item" 
			     hx-get="/messages/%d" 
				 hx-target="#chat-history"
				 hx-on::after-request="htmx.find('#chat-history').setAttribute('hx-get', '/messages/%d')"
				 onclick="document.querySelectorAll('.contact-item').forEach(el => el.classList.remove('bg-blue-100')); this.classList.add('bg-blue-100')">
				<div class="flex justify-between items-start">
					<p class="font-bold text-blue-800">%s %s</p>
					<span class="text-[10px] text-gray-400">@%s</span>
				</div>
				<p class="text-xs text-gray-600 truncate">%s</p>
			</div>
		`, c.ID, c.ID, c.FirstName, c.LastName, c.Username, lastMsg)
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

	if len(messages) == 0 {
		w.Write([]byte(`<p class="text-gray-500 italic p-4">No messages yet</p>`))
		return
	}

	fmt.Fprint(w, `<div class="flex flex-col space-y-2 p-2">`)
	for _, m := range messages {
		align := "items-start"
		bgColor := "bg-blue-100"
		if !m.IsIncoming {
			align = "items-end"
			bgColor = "bg-green-100"
		}
		fmt.Fprintf(w, `
			<div class="flex flex-col %s">
				<div class="%s p-3 rounded-lg max-w-[85%%] shadow-sm">
					<p class="text-sm text-gray-800">%s</p>
					<p class="text-[9px] text-gray-500 mt-1 text-right">%s</p>
				</div>
			</div>
		`, align, bgColor, m.Text, m.Timestamp.Format("Jan 02, 15:04"))
	}
	fmt.Fprint(w, `</div>`)
}
