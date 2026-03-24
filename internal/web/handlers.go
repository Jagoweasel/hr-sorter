package web

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"hr-sorter/internal/database"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/tgclient"
)

var tgManager *tgclient.Manager
var hhManager *hhclient.Manager
var rootCtx context.Context

func RegisterRoutes(mux *http.ServeMux, manager *tgclient.Manager, hhMan *hhclient.Manager, ctx context.Context) {
	tgManager = manager
	hhManager = hhMan
	rootCtx = ctx
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/contacts", handleContacts)
	mux.HandleFunc("/messages/", handleMessages)
	mux.HandleFunc("/accounts", handleAccounts)
	mux.HandleFunc("/accounts/create", handleCreateAccount)
	mux.HandleFunc("/accounts/toggle", handleToggleAccount)
	mux.HandleFunc("/accounts/delete", handleDeleteAccount)
	mux.HandleFunc("/integrations/create", handleCreateIntegration)
	mux.HandleFunc("/integrations/toggle", handleToggleIntegration)
	mux.HandleFunc("/integrations/delete", handleDeleteIntegration)
	mux.HandleFunc("/integrations/status", handleIntegrationStatus)
	mux.HandleFunc("/integrations/submit-code", handleSubmitCode)
	mux.HandleFunc("/integrations/submit-password", handleSubmitPassword)
	mux.HandleFunc("/integrations/submit-hh-code", handleSubmitHHCode)
	mux.HandleFunc("/pipeline", handlePipeline)
	mux.HandleFunc("/contacts/", handleContactActions) // Catch-all for /contacts/{id}/actions etc
	mux.HandleFunc("/sequences/create", handleCreateSequence)
	mux.HandleFunc("/sequences/add-contact", handleAddToSequence)
	mux.HandleFunc("/sequences/add-stage-modal", handleAddStageModal)
	mux.HandleFunc("/stages/update", handleUpdateStage)
	mux.HandleFunc("/stages/add", handleAddStage)
	mux.HandleFunc("/sequences/move", handleMoveSequence)
	mux.HandleFunc("/sequences/delete", handleDeleteSequence)
	mux.HandleFunc("/filters", handleGetFilters)
	mux.HandleFunc("/filters/add", handleAddFilter)
	mux.HandleFunc("/filters/delete", handleDeleteFilter)
	mux.HandleFunc("/filters/toggle", handleToggleFilter)
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	var accounts []models.Account
	database.DB.Select(&accounts, "SELECT * FROM accounts WHERE status = 'active' ORDER BY name")

	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/index.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", accounts)
}

func handleAccounts(w http.ResponseWriter, r *http.Request) {
	var accounts []models.Account
	err := database.DB.Select(&accounts, "SELECT * FROM accounts ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	type AccountWithIntegrations struct {
		models.Account
		Integrations []models.Integration
	}
	var data []AccountWithIntegrations
	for _, acc := range accounts {
		var ints []models.Integration
		database.DB.Select(&ints, "SELECT * FROM integrations WHERE account_id = ?", acc.ID)
		data = append(data, AccountWithIntegrations{acc, ints})
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/accounts.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	database.DB.MustExec("INSERT INTO accounts (name, status) VALUES (?, 'active')", name)
	http.Redirect(w, r, "/accounts", 303)
}

func handleToggleAccount(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var status string
	database.DB.Get(&status, "SELECT status FROM accounts WHERE id = ?", id)
	newStatus := "active"
	if status == "active" {
		newStatus = "inactive"
		// Stop all integrations for this account
		var integrations []models.Integration
		database.DB.Select(&integrations, "SELECT * FROM integrations WHERE account_id = ?", id)
		for _, i := range integrations {
			if i.Platform == "tg" {
				tgManager.StopIntegration(i.ID)
			} else if i.Platform == "hh" {
				hhManager.StopIntegration(i.ID)
			}
		}
	} else {
		// Restart integrations for this account
		var integrations []models.Integration
		database.DB.Select(&integrations, "SELECT * FROM integrations WHERE account_id = ?", id)
		for _, i := range integrations {
			if i.Status == "active" || i.Status == "pending_auth" {
				if i.Platform == "tg" {
					go tgManager.StartIntegration(rootCtx, i)
				} else if i.Platform == "hh" {
					go hhManager.StartIntegration(rootCtx, i)
				}
			}
		}
	}
	database.DB.MustExec("UPDATE accounts SET status = ? WHERE id = ?", newStatus, id)
	http.Redirect(w, r, "/accounts", 303)
}

func handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	// Clean up all integrations within this account first
	var integrations []models.Integration
	err := database.DB.Select(&integrations, "SELECT * FROM integrations WHERE account_id = ?", id)
	if err == nil {
		for _, i := range integrations {
			if i.Platform == "tg" {
				tgManager.StopIntegration(i.ID)
				if i.SessionPath != "" {
					os.Remove(i.SessionPath)
				}
			} else if i.Platform == "hh" {
				hhManager.StopIntegration(i.ID)
			}
		}
	}

	database.DB.MustExec("DELETE FROM accounts WHERE id = ?", id)
	http.Redirect(w, r, "/accounts", 303)
}

func handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	accID := r.FormValue("account_id")
	platform := r.FormValue("platform")
	identifier := r.FormValue("identifier")
	apiIDStr := r.FormValue("api_id")
	apiHash := r.FormValue("api_hash")

	apiID := 0
	if apiIDStr != "" {
		apiID, _ = strconv.Atoi(apiIDStr)
	}

	status := "pending_auth"
	sessionPath := ""
	var userAgent *string
	if platform == "tg" {
		sessionDir := "sessions"
		sessionPath = fmt.Sprintf("%s/%s.json", sessionDir, identifier)
	} else if platform == "hh" {
		ua := hhclient.GenerateAndroidUserAgent()
		userAgent = &ua
	}

	res, err := database.DB.Exec("INSERT INTO integrations (account_id, platform, identifier, api_id, api_hash, status, session_path, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		accID, platform, identifier, apiID, apiHash, status, sessionPath, userAgent)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if platform == "tg" {
		id, _ := res.LastInsertId()
		var integration models.Integration
		database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
		go tgManager.StartIntegration(rootCtx, integration)
	} else if platform == "hh" {
		id, _ := res.LastInsertId()
		var integration models.Integration
		database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
		logger.Debug(logger.HH, "[Web] Starting HH worker for new integration %s", identifier)
		go hhManager.StartIntegration(rootCtx, integration)
	}

	http.Redirect(w, r, "/accounts", 303)
}

func handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		http.Error(w, "Integration not found", 404)
		return
	}

	newStatus := "active"
	if integration.Status == "active" {
		newStatus = "inactive"
		if integration.Platform == "tg" {
			tgManager.StopIntegration(integration.ID)
		} else if integration.Platform == "hh" {
			hhManager.StopIntegration(integration.ID)
		}
	} else {
		if integration.Platform == "tg" {
			go tgManager.StartIntegration(rootCtx, integration)
		} else if integration.Platform == "hh" {
			go hhManager.StartIntegration(rootCtx, integration)
		}
	}

	database.DB.MustExec("UPDATE integrations SET status = ? WHERE id = ?", newStatus, id)
	http.Redirect(w, r, "/accounts", 303)
}

func handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		http.Redirect(w, r, "/accounts", 303)
		return
	}

	// 1. Stop background processing
	if integration.Platform == "tg" {
		tgManager.StopIntegration(integration.ID)
	} else if integration.Platform == "hh" {
		hhManager.StopIntegration(integration.ID)
	}

	// 2. Clean up filesystem (for TG)
	if integration.Platform == "tg" && integration.SessionPath != "" {
		if _, err := os.Stat(integration.SessionPath); err == nil {
			logger.Debug(logger.Telegram, "Deleting session file: %s", integration.SessionPath)
			os.Remove(integration.SessionPath)
		}
	}

	// 3. Delete from DB (cascades will handle contacts/messages/state)
	database.DB.MustExec("DELETE FROM integrations WHERE id = ?", id)
	http.Redirect(w, r, "/accounts", 303)
}

func handleIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "error"}`))
		return
	}

	authURL := ""
	if integration.Platform == "hh" && integration.Status == "pending_auth" {
		authURL = hhclient.GetAuthorizeURL()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "%s", "identifier": "%s", "platform": "%s", "auth_url": "%s"}`,
		integration.Status, integration.Identifier, integration.Platform, authURL)
}

func handleSubmitCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	idStr := r.FormValue("integration_id")
	code := r.FormValue("code")

	id, _ := strconv.ParseInt(idStr, 10, 64)

	ok := tgManager.SubmitCode(id, code)
	if ok {
		w.Write([]byte(`{"ok": true}`))
	} else {
		w.Write([]byte(`{"ok": false, "error": "no pending auth request"}`))
	}
}

func handleSubmitHHCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	idStr := r.FormValue("integration_id")
	code := r.FormValue("code")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	logger.Debug(logger.HH, "[Int ID %d] Received HH auth code submission", id)

	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "Integration not found: %v"}`, err)))
		return
	}

	ua := ""
	if integration.UserAgent != nil {
		ua = *integration.UserAgent
	}

	token, err := hhclient.ExchangeToken(code, ua)
	if err != nil {
		logger.Debug(logger.HH, "[Int ID %d] HH token exchange failed: %v", id, err)
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "%v"}`, err)))
		return
	}

	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	_, err = database.DB.Exec("UPDATE integrations SET access_token = ?, refresh_token = ?, expires_at = ?, status = 'active' WHERE id = ?",
		token.AccessToken, token.RefreshToken, expiresAt, id)
	if err != nil {
		logger.Debug(logger.HH, "[Int ID %d] HH DB update failed: %v", id, err)
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "DB error: %v"}`, err)))
		return
	}

	logger.Debug(logger.HH, "[Int ID %d] HH auth successful, triggering immediate sync", id)

	// Start integration in background manager
	database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", id)
	go hhManager.StartIntegration(rootCtx, integration)

	w.Write([]byte(`{"ok": true}`))
}

func handleSubmitPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	idStr := r.FormValue("integration_id")
	password := r.FormValue("password")

	id, _ := strconv.ParseInt(idStr, 10, 64)

	ok := tgManager.SubmitPassword(id, password)
	if ok {
		w.Write([]byte(`{"ok": true}`))
	} else {
		w.Write([]byte(`{"ok": false, "error": "no pending auth request"}`))
	}
}

func handleContacts(w http.ResponseWriter, r *http.Request) {
	activeAccountID := r.URL.Query().Get("account_id")
	platformFilter := r.URL.Query().Get("platform") // "tg", "hh", or empty for both
	showDeclines := r.URL.Query().Get("show_declines") == "true"
	hideScreened := r.URL.Query().Get("hide_screened") == "true"

	query := `
		SELECT c.*, 
		       COALESCE((SELECT text FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), '') as last_message,
			   COALESCE((SELECT datetime(timestamp) FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), datetime(c.created_at)) as last_time,
			   EXISTS(SELECT 1 FROM sequence_contacts WHERE contact_id = c.id) as in_sequence,
			   COALESCE((SELECT s.status FROM sequences s JOIN sequence_contacts sc ON s.id = sc.sequence_id WHERE sc.contact_id = c.id LIMIT 1), '') as seq_status
		FROM contacts c
		JOIN integrations i ON c.integration_id = i.id
		WHERE 1=1`

	args := []interface{}{}
	if activeAccountID != "" {
		query += " AND i.account_id = ?"
		args = append(args, activeAccountID)
	}
	if platformFilter != "" {
		query += " AND c.platform = ?"
		args = append(args, platformFilter)
	}
	if !showDeclines {
		// Filter out HH declines
		query += " AND NOT (c.platform = 'hh' AND (c.username = 'Отказ' OR c.username = 'discard'))"
	}
	query += " ORDER BY last_time DESC"

	deref := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}

	// Normalizer for whitespace and casing
	normalize := func(s string) string {
		// Replace all whitespace (newlines, tabs, various Unicode spaces) with a single space
		f := func(r rune) bool {
			return unicode.IsSpace(r)
		}
		words := strings.FieldsFunc(s, f)
		return strings.ToLower(strings.Join(words, " "))
	}

	type ContactWithLastMsg struct {
		models.Contact
		LastMessage string `db:"last_message"`
		LastTime    string `db:"last_time"`
	}
	var allContacts []ContactWithLastMsg
	err := database.DB.Select(&allContacts, query, args...)
	if err != nil {
		log.Printf("Web: Error fetching contacts: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	// Load active filters if screening is requested
	var activePatterns []string
	if hideScreened {
		var filters []models.MessageFilter
		database.DB.Select(&filters, "SELECT pattern FROM message_filters WHERE is_active = 1")
		for _, f := range filters {
			activePatterns = append(activePatterns, normalize(f.Pattern))
		}
	}

	var filteredContacts []ContactWithLastMsg
	for _, c := range allContacts {
		if hideScreened && c.Platform == "hh" && len(activePatterns) > 0 {
			normMsg := normalize(c.LastMessage)
			isScreened := false
			for _, p := range activePatterns {
				if strings.Contains(normMsg, p) {
					isScreened = true
					break
				}
			}
			if isScreened {
				continue
			}
		}
		filteredContacts = append(filteredContacts, c)
	}

	for _, c := range filteredContacts {
		lastMsg := strings.ReplaceAll(c.LastMessage, "\n", " ")
		if lastMsg == "" {
			// Fallback: show status if no messages
			lastMsg = "[" + deref(c.Username) + "]"
		}

		if len(lastMsg) > 40 {
			lastMsg = lastMsg[:37] + "..."
		}

		statusIndicator := ""
		if c.InSequence {
			color := "bg-gray-400"
			switch c.SeqStatus {
			case "accepted":
				color = "bg-green-500"
			case "rejected":
				color = "bg-red-500"
			case "offer":
				color = "bg-yellow-500"
			default:
				color = "bg-blue-500"
			}
			statusIndicator = fmt.Sprintf(`<span class="w-2 h-2 rounded-full %s ml-2"></span>`, color)
		}

		platformIcon := `<span class="text-blue-400" title="Telegram">
			<svg class="w-3 h-3" fill="currentColor" viewBox="0 0 24 24"><path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm4.64 6.8c-.15 1.58-.8 5.42-1.13 7.19-.14.75-.42 1-.68 1.03-.58.05-1.02-.38-1.58-.75-.88-.58-1.38-.94-2.23-1.5-.99-.65-.35-1.01.22-1.59.15-.15 2.71-2.48 2.76-2.69.01-.03.01-.14-.07-.2-.08-.06-.19-.04-.27-.02-.12.02-1.96 1.25-5.54 3.69-.52.35-.97.52-1.35.51-.42-.01-1.24-.24-1.84-.44-.74-.24-1.33-.37-1.28-.79.03-.22.33-.44.89-.67 3.49-1.52 5.82-2.52 6.99-3.01 3.32-1.39 4.02-1.63 4.47-1.63.1 0 .32.02.46.14.12.1.15.23.16.33.01.07.02.21.01.35z"/></svg>
		</span>`

		nameDisplay := fmt.Sprintf("%s %s", deref(c.FirstName), deref(c.LastName))
		subDisplay := "@" + deref(c.Username)

		if c.Platform == "hh" {
			platformIcon = `<span class="text-red-500 font-black text-[10px] border border-red-200 px-1 rounded bg-red-50" title="HeadHunter">HH</span>`
			nameDisplay = deref(c.FirstName) // Employer Name
			subDisplay = deref(c.LastName)   // Vacancy Name
			// Add HH status badge
			statusIndicator += fmt.Sprintf(`<span class="ml-2 px-1.5 py-0.5 rounded-full bg-gray-100 text-gray-600 text-[8px] font-black uppercase tracking-tighter">%s</span>`, deref(c.Username))
		}

		fmt.Fprintf(w, `
			<div class="p-3 border-b cursor-pointer hover:bg-blue-50 transition-colors contact-item group relative" 
			     hx-get="/messages/%d" 
				 hx-target="#chat-history"
				 onclick="document.querySelectorAll('.contact-item').forEach(el => el.classList.remove('bg-blue-100')); this.classList.add('bg-blue-100')">
				<div class="flex justify-between items-start">
					<div class="flex items-center space-x-2 overflow-hidden">
						%s
						<p class="font-bold text-blue-800 text-sm truncate">%s</p>
						%s
					</div>
					<span class="text-[10px] text-gray-400 shrink-0 ml-2">%s</span>
				</div>
				<p class="text-[11px] text-gray-600 truncate mt-1">%s</p>
				
				<button class="absolute right-2 bottom-2 opacity-0 group-hover:opacity-100 p-1 hover:bg-gray-200 rounded transition-opacity"
				        hx-get="/contacts/%d/actions"
						hx-target="#modal-container"
						onclick="event.stopPropagation()">
					<svg class="w-4 h-4 text-gray-500" fill="currentColor" viewBox="0 0 20 20"><path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z"></path></svg>
				</button>
			</div>
		`, c.ID, platformIcon, nameDisplay, statusIndicator, subDisplay, html.EscapeString(lastMsg), c.ID)
	}
}

func handleMessages(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/messages/")
	var messages []models.Message
	err := database.DB.Select(&messages, "SELECT * FROM messages WHERE contact_id = ? ORDER BY timestamp ASC", id)
	if err != nil {
		log.Printf("Web: Error fetching messages for contact %s: %v", id, err)
		http.Error(w, err.Error(), 500)
		return
	}

	log.Printf("Web: Rendering %d messages for contact %s", len(messages), id)

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

		// Parse the timestamp string
		displayTime := m.Timestamp
		t, err := time.Parse("2006-01-02 15:04:05", m.Timestamp)
		if err == nil {
			displayTime = t.Format("Jan 02, 15:04")
		} else {
			// Try RFC3339 as fallback
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

type SequenceWithDetails struct {
	models.Sequence
	Recruiters []models.Contact
	Stages     []models.InterviewStage
	History    []models.InterviewStage
	IsRejected bool
	IsAccepted bool
}

type ColumnDef struct {
	ID          string
	Label       string
	ColorClass  string
	BorderClass string
}

type AccountGroup struct {
	Account   *models.Account
	Columns   []PipelineColumn
	Sequences []SequenceWithDetails
}

type PipelineColumn struct {
	ColumnDef
	Sequences []SequenceWithDetails
}

func handlePipeline(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "kanban"
	}

	activeAccountID := r.URL.Query().Get("account_id")

	var allAccounts []models.Account
	database.DB.Select(&allAccounts, "SELECT * FROM accounts ORDER BY name")

	var sequences []models.Sequence
	query := "SELECT * FROM sequences"
	args := []interface{}{}
	if activeAccountID != "" {
		query += " WHERE account_id = ?"
		args = append(args, activeAccountID)
	}
	query += " ORDER BY created_at DESC"

	err := database.DB.Select(&sequences, query, args...)
	if err != nil {
		log.Printf("Web: Error fetching sequences: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	var detailedSeqs []SequenceWithDetails
	for _, s := range sequences {
		var recruiters []models.Contact
		err = database.DB.Select(&recruiters, `
			SELECT c.* FROM contacts c 
			JOIN sequence_contacts sc ON c.id = sc.contact_id 
			WHERE sc.sequence_id = ?`, s.ID)
		if err != nil {
			logger.Debug(logger.History, "Error fetching recruiters for Seq #%d: %v", s.ID, err)
		}

		var stages []models.InterviewStage
		err = database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? ORDER BY order_index ASC", s.ID)
		if err != nil {
			logger.Debug(logger.History, "Error fetching stages for Seq #%d: %v", s.ID, err)
		}

		var history []models.InterviewStage
		var historyNames []string
		for _, st := range stages {
			logger.Debug(logger.History, "  - Stage: '%s', Completed: %v, Order: %d", st.Name, st.IsCompleted, st.OrderIndex)
			if st.IsCompleted {
				history = append(history, st)
				historyNames = append(historyNames, st.Name)
			}
		}

		detailedSeqs = append(detailedSeqs, SequenceWithDetails{
			Sequence:   s,
			Recruiters: recruiters,
			Stages:     stages,
			History:    history,
			IsRejected: s.Status == "rejected",
			IsAccepted: s.Status == "accepted",
		})

		logger.Debug(logger.History, "Seq #%d (%s): Found %d total stages, %d history stages (Status: %s)", s.ID, s.CompanyName, len(stages), len(history), s.Status)
		logger.LogChain(s.ID, s.CompanyName, historyNames, s.Status)
	}

	columnDefs := []ColumnDef{
		{ID: "initial", Label: "Initial", ColorClass: "bg-blue-50", BorderClass: "border-blue-200"},
		{ID: "screening", Label: "Screening", ColorClass: "bg-indigo-50", BorderClass: "border-indigo-200"},
		{ID: "tech", Label: "Technical", ColorClass: "bg-purple-50", BorderClass: "border-purple-200"},
		{ID: "final", Label: "Final Interview", ColorClass: "bg-pink-50", BorderClass: "border-pink-200"},
		{ID: "offer", Label: "Offer", ColorClass: "bg-yellow-50", BorderClass: "border-yellow-200"},
		{ID: "accepted", Label: "Accepted", ColorClass: "bg-green-50", BorderClass: "border-green-200"},
		{ID: "rejected", Label: "Rejected", ColorClass: "bg-red-50", BorderClass: "border-red-200"},
	}

	// Create account groups
	var groups []AccountGroup

	// Group for sequences without an account
	orphanGroup := AccountGroup{
		Account: nil,
	}
	for _, def := range columnDefs {
		orphanGroup.Columns = append(orphanGroup.Columns, PipelineColumn{ColumnDef: def})
	}

	// Groups for each account
	accountMap := make(map[int64]*AccountGroup)
	for i := range allAccounts {
		acc := allAccounts[i]
		group := AccountGroup{
			Account: &acc,
		}
		for _, def := range columnDefs {
			group.Columns = append(group.Columns, PipelineColumn{ColumnDef: def})
		}
		groups = append(groups, group)
		accountMap[acc.ID] = &groups[len(groups)-1]
	}

	// Distribute sequences
	for _, s := range detailedSeqs {
		var targetGroup *AccountGroup
		if s.AccountID != nil {
			targetGroup = accountMap[*s.AccountID]
		}
		if targetGroup == nil {
			targetGroup = &orphanGroup
		}

		targetGroup.Sequences = append(targetGroup.Sequences, s)
		for i := range targetGroup.Columns {
			if s.Status == targetGroup.Columns[i].ID {
				targetGroup.Columns[i].Sequences = append(targetGroup.Columns[i].Sequences, s)
			}
		}
	}

	// Only add orphan group if it has sequences
	if len(orphanGroup.Sequences) > 0 {
		groups = append(groups, orphanGroup)
	}

	data := struct {
		View       string
		Groups     []AccountGroup
		ColumnDefs []ColumnDef
	}{
		View:       view,
		Groups:     groups,
		ColumnDefs: columnDefs,
	}

	tmpl := template.New("layout.html").Funcs(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"sub": func(a, b int) int {
			return a - b
		},
		"lastStage": func(history []models.InterviewStage) string {
			if len(history) == 0 {
				return "None"
			}
			return history[len(history)-1].Name
		},
		"slice": func(s string, start, end int) string {
			if len(s) < end {
				return s[start:]
			}
			return s[start:end]
		},
	})
	tmpl = template.Must(tmpl.ParseFiles(
		"templates/layout.html",
		"templates/pipeline.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", data)
}

func handleContactActions(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/actions") {
		id := strings.TrimPrefix(strings.TrimSuffix(path, "/actions"), "/contacts/")
		fmt.Fprintf(w, `
			<div class="fixed inset-0 bg-black/20 backdrop-blur-[1px] flex items-center justify-center z-[100]" id="modal" onclick="if(event.target === this) htmx.remove('#modal')">
				<div class="bg-white border shadow-2xl rounded-2xl p-2 w-64 overflow-hidden" onclick="event.stopPropagation()">
					<div class="px-4 py-3 border-b mb-1">
						<p class="text-[10px] font-black text-gray-400 uppercase tracking-widest">Recruiter Actions</p>
					</div>
					<button class="w-full text-left px-4 py-3 hover:bg-blue-50 text-blue-700 font-bold text-sm transition-colors rounded-xl flex items-center justify-between group"
					        hx-get="/contacts/%s/create-sequence-modal"
							hx-target="#modal-container"
							hx-on:htmx:after-request="htmx.remove('#modal')">
						Start Sequence
						<span class="opacity-0 group-hover:opacity-100 transition-opacity">→</span>
					</button>
					<button class="w-full text-left px-4 py-3 hover:bg-blue-50 text-gray-700 font-bold text-sm transition-colors rounded-xl flex items-center justify-between group"
					        hx-get="/contacts/%s/add-to-sequence-modal"
							hx-target="#modal-container"
							hx-on:htmx:after-request="htmx.remove('#modal')">
						Add to Existing
						<span class="opacity-0 group-hover:opacity-100 transition-opacity">→</span>
					</button>
					<div class="p-2 border-t mt-1">
						<button onclick="htmx.remove('#modal')" class="w-full py-2 text-[10px] font-black text-gray-400 uppercase hover:text-gray-600 transition-colors">Cancel</button>
					</div>
				</div>
			</div>
		`, id, id)
		return
	}

	if strings.HasSuffix(path, "/create-sequence-modal") {
		contactID := strings.TrimPrefix(strings.TrimSuffix(path, "/create-sequence-modal"), "/contacts/")

		// Get earliest message date
		var firstMsgDate string
		err := database.DB.Get(&firstMsgDate, "SELECT COALESCE(datetime(MIN(timestamp)), datetime('now')) FROM messages WHERE contact_id = ?", contactID)
		if err != nil {
			firstMsgDate = time.Now().Format("2006-01-02T15:04")
		} else {
			// Convert to HTML datetime-local format
			t, _ := time.Parse("2006-01-02 15:04:05", firstMsgDate)
			firstMsgDate = t.Format("2006-01-02T15:04")
		}

		fmt.Fprintf(w, `
			<div class="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-[110] overflow-y-auto p-4" id="modal" onclick="if(event.target === this) htmx.remove('#modal')">
				<div class="bg-white rounded-3xl shadow-2xl p-8 w-full max-w-lg my-auto relative" onclick="event.stopPropagation()">
					<div class="flex justify-between items-center mb-6 border-b pb-4">
						<h2 class="text-2xl font-black text-gray-900 tracking-tight">New Sequence</h2>
						<button onclick="htmx.remove('#modal')" class="text-gray-400 hover:text-gray-600">
							<svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"></path></svg>
						</button>
					</div>
					<form hx-post="/sequences/create" hx-target="body">
						<input type="hidden" name="contact_id" value="%s">
						<div class="space-y-6">
							<div class="grid grid-cols-2 gap-6">
								<div>
									<label class="block text-[10px] font-black text-gray-400 uppercase tracking-[0.2em] mb-2">Company</label>
									<input type="text" name="company_name" required class="block w-full border-2 border-gray-100 rounded-xl p-3 focus:border-blue-500 focus:ring-0 transition-colors text-sm font-bold bg-gray-50" placeholder="Google">
								</div>
								<div>
									<label class="block text-[10px] font-black text-gray-400 uppercase tracking-[0.2em] mb-2">Vacancy</label>
									<input type="text" name="vacancy_name" value="Senior Go Dev" required class="block w-full border-2 border-gray-100 rounded-xl p-3 focus:border-blue-500 focus:ring-0 transition-colors text-sm font-bold bg-gray-50" placeholder="Senior Go Dev">
								</div>
							</div>
							<div>
								<label class="block text-[10px] font-black text-gray-400 uppercase tracking-[0.2em] mb-2">Initial Contact Date</label>
								<input type="datetime-local" name="initial_date" value="%s" class="block w-full border-2 border-gray-100 rounded-xl p-3 focus:border-blue-500 focus:ring-0 text-sm font-bold bg-gray-50">
							</div>
						</div>
						<div class="mt-10 flex justify-end space-x-4">
							<button type="button" onclick="htmx.remove('#modal')" class="px-6 py-3 text-gray-400 hover:text-gray-600 text-xs font-black uppercase tracking-widest">Cancel</button>
							<button type="submit" class="px-8 py-3 bg-gray-900 text-white rounded-xl hover:bg-black font-black text-xs uppercase tracking-[0.2em] shadow-xl hover:-translate-y-0.5 transition-all">Create</button>
						</div>
					</form>
				</div>
			</div>
		`, contactID, firstMsgDate)
		return
	}

	if strings.HasSuffix(path, "/add-to-sequence-modal") {
		contactID := strings.TrimPrefix(strings.TrimSuffix(path, "/add-to-sequence-modal"), "/contacts/")
		var sequences []models.Sequence
		database.DB.Select(&sequences, "SELECT * FROM sequences WHERE status NOT IN ('accepted', 'rejected') ORDER BY created_at DESC")

		options := ""
		for _, s := range sequences {
			options += fmt.Sprintf(`<option value="%d">%s — %s</option>`, s.ID, s.CompanyName, s.VacancyName)
		}

		fmt.Fprintf(w, `
			<div class="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-[110]" id="modal" onclick="if(event.target === this) htmx.remove('#modal')">
				<div class="bg-white rounded-3xl shadow-2xl p-8 w-full max-w-md relative" onclick="event.stopPropagation()">
					<div class="flex justify-between items-center mb-6">
						<h2 class="text-xl font-black text-gray-900 tracking-tight">Link Recruiter</h2>
					</div>
					<form hx-post="/sequences/add-contact" hx-target="body">
						<input type="hidden" name="contact_id" value="%s">
						<div class="space-y-4">
							<label class="block text-[10px] font-black text-gray-400 uppercase tracking-widest">Select Active Sequence</label>
							<select name="sequence_id" required class="block w-full border-2 border-gray-100 rounded-xl p-3 text-sm font-bold bg-gray-50 focus:border-blue-500 outline-none transition-colors">
								%s
							</select>
						</div>
						<div class="mt-8 flex justify-end space-x-3">
							<button type="button" onclick="htmx.remove('#modal')" class="px-5 py-2 text-[10px] font-black text-gray-400 uppercase tracking-widest">Cancel</button>
							<button type="submit" class="px-6 py-2 bg-blue-600 text-white rounded-xl font-black text-[10px] uppercase tracking-[0.2em] shadow-lg hover:bg-blue-700 transition-colors">Confirm Link</button>
						</div>
					</form>
				</div>
			</div>
		`, contactID, options)
		return
	}
}

func handleAddStageModal(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("sequence_id")

	fmt.Fprintf(w, `
		<div class="fixed inset-0 bg-black/50 backdrop-blur-sm flex items-center justify-center z-[110]" id="modal" onclick="if(event.target === this) htmx.remove('#modal')">
			<div class="bg-white rounded-3xl shadow-2xl p-8 w-full max-w-xs relative" onclick="event.stopPropagation()">
				<h2 class="text-[10px] font-black text-gray-400 uppercase tracking-[0.2em] mb-6 text-center">New Workflow Step</h2>
				<div class="grid grid-cols-1 gap-2">
					<button hx-get="/stages/add?sequence_id=%s&category=screening" hx-target="body" 
					        class="px-4 py-3 bg-indigo-50 text-indigo-700 hover:bg-indigo-100 rounded-xl text-xs font-black uppercase tracking-widest transition-colors">
						Screening +
					</button>
					<button hx-get="/stages/add?sequence_id=%s&category=tech" hx-target="body"
					        class="px-4 py-3 bg-purple-50 text-purple-700 hover:bg-purple-100 rounded-xl text-xs font-black uppercase tracking-widest transition-colors">
						Technical +
					</button>
					<button hx-get="/stages/add?sequence_id=%s&category=final" hx-target="body"
					        class="px-4 py-3 bg-pink-50 text-pink-700 hover:bg-pink-100 rounded-xl text-xs font-black uppercase tracking-widest transition-colors">
						Final Interview +
					</button>
					<button hx-get="/stages/add?sequence_id=%s&category=offer" hx-target="body"
					        class="px-4 py-3 bg-yellow-50 text-yellow-700 hover:bg-yellow-100 rounded-xl text-xs font-black uppercase tracking-widest transition-colors">
						Offer +
					</button>
				</div>
				<button onclick="htmx.remove('#modal')" class="w-full mt-6 py-2 text-[9px] font-black text-gray-400 hover:text-gray-600 uppercase tracking-widest">Close</button>
			</div>
		</div>
	`, seqID, seqID, seqID, seqID)
}

func handleCreateSequence(w http.ResponseWriter, r *http.Request) {
	logger.Debug(logger.AddSequence, "handleCreateSequence triggered")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	company := r.FormValue("company_name")
	vacancy := r.FormValue("vacancy_name")
	contactID := r.FormValue("contact_id")
	initialDateStr := r.FormValue("initial_date")

	logger.Debug(logger.AddSequence, "Creating sequence for Company='%s', Vacancy='%s', ContactID='%s'", company, vacancy, contactID)

	tx, err := database.DB.Beginx()
	if err != nil {
		logger.Debug(logger.AddSequence, "Error starting transaction: %v", err)
		http.Error(w, "Database error", 500)
		return
	}
	defer tx.Rollback()

	// Find the account associated with this contact
	var accountID *int64
	var foundID int64
	err = tx.Get(&foundID, `
		SELECT i.account_id FROM messages m 
		JOIN integrations i ON m.integration_id = i.id 
		WHERE m.contact_id = ? AND i.account_id IS NOT NULL LIMIT 1
	`, contactID)
	if err == nil {
		accountID = &foundID
		logger.Debug(logger.AddSequence, "Found account ID %d from previous messages", foundID)
	} else {
		err = tx.Get(&foundID, "SELECT id FROM accounts WHERE status = 'active' LIMIT 1")
		if err == nil {
			accountID = &foundID
			logger.Debug(logger.AddSequence, "Using first active account ID %d", foundID)
		} else {
			err = tx.Get(&foundID, "SELECT id FROM accounts LIMIT 1")
			if err == nil {
				accountID = &foundID
				logger.Debug(logger.AddSequence, "Fallback to any account ID %d", foundID)
			}
		}
	}

	res, err := tx.Exec("INSERT INTO sequences (account_id, company_name, vacancy_name, status) VALUES (?, ?, ?, ?)", accountID, company, vacancy, "initial")
	if err != nil {
		logger.Debug(logger.AddSequence, "Error inserting sequence: %v", err)
		return
	}

	seqID, _ := res.LastInsertId()
	logger.Debug(logger.AddSequence, "Sequence created with ID %d", seqID)

	if contactID != "" {
		_, err = tx.Exec("INSERT INTO sequence_contacts (sequence_id, contact_id) VALUES (?, ?)", seqID, contactID)
		if err != nil {
			logger.Debug(logger.AddSequence, "Error linking contact: %v", err)
		}
	}

	// Create initial stage
	initialDate, _ := time.Parse("2006-01-02T15:04", initialDateStr)
	_, err = tx.Exec("INSERT INTO interview_stages (sequence_id, name, scheduled_at, is_completed, order_index) VALUES (?, ?, ?, ?, ?)",
		seqID, "Initial Contact", initialDate, 1, 0)
	if err != nil {
		logger.Debug(logger.AddSequence, "Error creating initial stage: %v", err)
	} else {
		logger.Debug(logger.AddSequence, "Created 'Initial Contact' stage (Completed: true, Order: 0)")
	}

	// Add HR Screening
	_, err = tx.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)",
		seqID, "HR Screening", 1)
	if err != nil {
		logger.Debug(logger.AddSequence, "Error creating screening stage: %v", err)
	} else {
		logger.Debug(logger.AddSequence, "Created 'HR Screening' stage (Completed: false, Order: 1)")
	}

	// Add default final stages
	tx.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)", seqID, "Final Interview", 2)
	tx.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)", seqID, "Offer", 3)
	logger.Debug(logger.AddSequence, "Created Final and Offer stages")

	if err := tx.Commit(); err != nil {
		logger.Debug(logger.AddSequence, "Error committing transaction: %v", err)
		http.Error(w, "Commit failed", 500)
		return
	}

	logger.Debug(logger.AddSequence, "Transaction committed successfully for sequence %d", seqID)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/pipeline")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/pipeline", 303)
}

func handleAddToSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	seqID := r.FormValue("sequence_id")
	contactID := r.FormValue("contact_id")

	_, err := database.DB.Exec("INSERT OR IGNORE INTO sequence_contacts (sequence_id, contact_id) VALUES (?, ?)", seqID, contactID)
	if err != nil {
		logger.Debug(logger.AddSequence, "Error adding contact %s to sequence %s: %v", contactID, seqID, err)
		http.Error(w, err.Error(), 500)
		return
	}

	logger.Debug(logger.AddSequence, "Added recruiter ID %s to sequence ID %s", contactID, seqID)

	// Log current chain
	var seq models.Sequence
	database.DB.Get(&seq, "SELECT * FROM sequences WHERE id = ?", seqID)
	var stages []models.InterviewStage
	database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index ASC", seqID)
	var names []string
	for _, s := range stages {
		names = append(names, s.Name)
	}
	logger.LogChain(seq.ID, seq.CompanyName, names, seq.Status)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/", 303)
}

func getRedirectURL(r *http.Request) string {
	referer := r.Header.Get("Referer")
	if strings.Contains(referer, "view=timeline") {
		return "/pipeline?view=timeline"
	}
	return "/pipeline"
}

func handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	stageID := r.URL.Query().Get("id")
	completed := r.URL.Query().Get("completed") == "true"

	var stage models.InterviewStage
	err := database.DB.Get(&stage, "SELECT * FROM interview_stages WHERE id = ?", stageID)
	if err != nil {
		http.Error(w, "Stage not found", 404)
		return
	}

	if !completed {
		// If unmarking a stage, check if it's a custom stage.
		// Standard stages are NOT deleted.
		standard := false
		lowerName := strings.ToLower(stage.Name)
		standards := []string{"initial contact", "hr screening", "technical interview 1", "final interview", "offer"}
		for _, s := range standards {
			if lowerName == s {
				standard = true
				break
			}
		}

		if !standard {
			database.DB.Exec("DELETE FROM interview_stages WHERE id = ?", stageID)
			logger.Debug(logger.History, "Deleted custom stage ID %s ('%s') because it was unmarked", stageID, stage.Name)
		} else {
			database.DB.Exec("UPDATE interview_stages SET is_completed = 0 WHERE id = ?", stageID)
		}
	} else {
		database.DB.Exec("UPDATE interview_stages SET is_completed = 1 WHERE id = ?", stageID)
	}

	logger.Debug(logger.History, "Updated stage ID %s to completed=%v", stageID, completed)

	// Update sequence status based on the last completed stage
	var stages []models.InterviewStage
	database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index DESC LIMIT 1", stage.SequenceID)

	if len(stages) > 0 {
		s := stages[0]
		name := strings.ToLower(s.Name)
		newStatus := ""
		if strings.Contains(name, "offer") {
			newStatus = "offer"
		} else if strings.Contains(name, "final") {
			newStatus = "final"
		} else if strings.Contains(name, "tech") {
			newStatus = "tech"
		} else if strings.Contains(name, "screen") {
			newStatus = "screening"
		} else {
			newStatus = "initial"
		}
		if newStatus != "" {
			database.DB.Exec("UPDATE sequences SET status = ? WHERE id = ?", newStatus, stage.SequenceID)
			logger.Debug(logger.History, "Sequence ID %d status automatically updated to '%s'", stage.SequenceID, newStatus)
		}
	} else {
		// No completed stages, revert to initial
		database.DB.Exec("UPDATE sequences SET status = 'initial' WHERE id = ?", stage.SequenceID)
	}

	// Log result chain
	var seq models.Sequence
	database.DB.Get(&seq, "SELECT * FROM sequences WHERE id = ?", stage.SequenceID)
	var updatedStages []models.InterviewStage
	database.DB.Select(&updatedStages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index ASC", stage.SequenceID)
	var names []string
	for _, s := range updatedStages {
		names = append(names, s.Name)
	}
	logger.LogChain(seq.ID, seq.CompanyName, names, seq.Status)

	http.Redirect(w, r, getRedirectURL(r), 303)
}

func handleAddStage(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("sequence_id")
	category := r.URL.Query().Get("category") // screening, tech, final, offer
	name := r.URL.Query().Get("name")

	if name == "" {
		label := category
		if category == "tech" {
			label = "Technical Interview"
		} else if category == "screening" {
			label = "HR Screening"
		} else if category == "final" {
			label = "Final Interview"
		} else if category == "offer" {
			label = "Offer"
		}

		var count int
		database.DB.Get(&count, "SELECT count(*) FROM interview_stages WHERE sequence_id = ? AND (name LIKE ? OR name LIKE ?)",
			seqID, label+"%", strings.Title(category)+"%")
		name = fmt.Sprintf("%s %d", strings.Title(label), count+1)
	}

	// Determine insertion point based on hierarchy
	var stages []models.InterviewStage
	database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? ORDER BY order_index ASC", seqID)

	hierarchy := map[string]int{
		"initial":   0,
		"screening": 1,
		"tech":      2,
		"final":     3,
		"offer":     4,
	}
	newRank := hierarchy[category]

	insertAt := 0
	if len(stages) > 0 {
		insertAt = stages[len(stages)-1].OrderIndex + 1
		for _, s := range stages {
			sName := strings.ToLower(s.Name)
			sRank := 0
			if strings.Contains(sName, "offer") {
				sRank = 4
			} else if strings.Contains(sName, "final") {
				sRank = 3
			} else if strings.Contains(sName, "tech") {
				sRank = 2
			} else if strings.Contains(sName, "screen") {
				sRank = 1
			}

			if sRank <= newRank {
				insertAt = s.OrderIndex + 1
			} else {
				// We found a stage that should come after the new one
				insertAt = s.OrderIndex
				break
			}
		}
	}

	// Shift subsequent stages
	database.DB.Exec("UPDATE interview_stages SET order_index = order_index + 1 WHERE sequence_id = ? AND order_index >= ?", seqID, insertAt)

	_, err := database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, is_completed, order_index) VALUES (?, ?, 1, ?)",
		seqID, name, insertAt)

	if err == nil {
		logger.Debug(logger.History, "Manually added stage '%s' to sequence %s at index %d", name, seqID, insertAt)
		// Update sequence status
		status := category
		if category == "initial" {
			status = "initial"
		}
		database.DB.Exec("UPDATE sequences SET status = ? WHERE id = ?", status, seqID)

		// Log result chain
		var seq models.Sequence
		database.DB.Get(&seq, "SELECT * FROM sequences WHERE id = ?", seqID)
		var updatedStages []models.InterviewStage
		database.DB.Select(&updatedStages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index ASC", seqID)
		var names []string
		for _, s := range updatedStages {
			names = append(names, s.Name)
		}
		logger.LogChain(seq.ID, seq.CompanyName, names, seq.Status)
	}

	http.Redirect(w, r, getRedirectURL(r), 303)
}

func handleMoveSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	logger.Debug(logger.History, "handleMoveSequence: ID=%s, TargetStatus=%s (SURGICAL MOVE)", seqID, status)

	database.DB.Exec("UPDATE sequences SET status = ? WHERE id = ?", status, seqID)

	// If moving to Accepted/Rejected, we still might want some automation
	if status == "accepted" {
		// Just update status, don't force mark all stages
	} else if status == "rejected" {
		// Mark current point of rejection
		var firstIncomplete models.InterviewStage
		err := database.DB.Get(&firstIncomplete, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 0 ORDER BY order_index ASC LIMIT 1", seqID)
		if err == nil {
			database.DB.Exec("UPDATE interview_stages SET is_completed = 1 WHERE id = ?", firstIncomplete.ID)
		}
	}

	// Log result chain
	var seq models.Sequence
	database.DB.Get(&seq, "SELECT * FROM sequences WHERE id = ?", seqID)
	var updatedStages []models.InterviewStage
	database.DB.Select(&updatedStages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index ASC", seqID)
	var names []string
	for _, s := range updatedStages {
		names = append(names, s.Name)
	}
	logger.LogChain(seq.ID, seq.CompanyName, names, seq.Status)

	http.Redirect(w, r, getRedirectURL(r), 303)
}

func handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")

	var seq models.Sequence
	database.DB.Get(&seq, "SELECT * FROM sequences WHERE id = ?", seqID)

	database.DB.Exec("DELETE FROM sequences WHERE id = ?", seqID)
	logger.Debug(logger.AddSequence, "Deleted sequence ID %s (%s)", seqID, seq.CompanyName)

	http.Redirect(w, r, getRedirectURL(r), 303)
}

func handleGetFilters(w http.ResponseWriter, r *http.Request) {
	var filters []models.MessageFilter
	err := database.DB.Select(&filters, "SELECT * FROM message_filters ORDER BY created_at DESC")
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte(`<div class="space-y-3">`))
	for _, f := range filters {
		activeClass := "bg-green-100 text-green-700"
		if !f.IsActive {
			activeClass = "bg-gray-100 text-gray-500"
		}
		fmt.Fprintf(w, `
			<div class="flex items-center justify-between p-3 bg-gray-50 rounded-xl border border-gray-100 group">
				<div class="flex items-center space-x-3">
					<button hx-post="/filters/toggle?id=%d" hx-target="#filter-list-content" class="px-2 py-0.5 rounded text-[8px] font-black uppercase tracking-widest %s">
						%s
					</button>
					<span class="text-sm font-bold text-gray-700">%s</span>
				</div>
				<button hx-post="/filters/delete?id=%d" hx-target="#filter-list-content" hx-confirm="Delete pattern?" class="opacity-0 group-hover:opacity-100 p-1 text-red-400 hover:bg-red-50 rounded-lg transition-all">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
				</button>
			</div>
		`, f.ID, activeClass, map[bool]string{true: "Active", false: "Off"}[f.IsActive], html.EscapeString(f.Pattern), f.ID)
	}
	if len(filters) == 0 {
		w.Write([]byte(`<p class="text-center text-gray-400 italic text-sm py-4">No filters defined yet</p>`))
	}
	w.Write([]byte(`</div>`))
}

func handleAddFilter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}
	pattern := r.FormValue("pattern")
	if pattern != "" {
		database.DB.Exec("INSERT INTO message_filters (pattern) VALUES (?)", pattern)
	}
	handleGetFilters(w, r)
}

func handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	database.DB.Exec("DELETE FROM message_filters WHERE id = ?", id)
	handleGetFilters(w, r)
}

func handleToggleFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	database.DB.Exec("UPDATE message_filters SET is_active = NOT is_active WHERE id = ?", id)
	handleGetFilters(w, r)
}
