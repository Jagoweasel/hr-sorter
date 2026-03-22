package web

import (
	"fmt"
	"html"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"hr-sorter/internal/database"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
)

func RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/contacts", handleContacts)
	mux.HandleFunc("/messages/", handleMessages)
	mux.HandleFunc("/accounts", handleAccounts)
	mux.HandleFunc("/pipeline", handlePipeline)
	mux.HandleFunc("/contacts/", handleContactActions) // Catch-all for /contacts/{id}/actions etc
	mux.HandleFunc("/sequences/create", handleCreateSequence)
	mux.HandleFunc("/sequences/add-contact", handleAddToSequence)
	mux.HandleFunc("/sequences/add-stage-modal", handleAddStageModal)
	mux.HandleFunc("/stages/update", handleUpdateStage)
	mux.HandleFunc("/stages/add", handleAddStage)
	mux.HandleFunc("/sequences/move", handleMoveSequence)
	mux.HandleFunc("/sequences/delete", handleDeleteSequence)
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
			   COALESCE((SELECT datetime(timestamp) FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), datetime(c.created_at)) as last_time,
			   EXISTS(SELECT 1 FROM sequence_contacts WHERE contact_id = c.id) as in_sequence,
			   COALESCE((SELECT s.status FROM sequences s JOIN sequence_contacts sc ON s.id = sc.sequence_id WHERE sc.contact_id = c.id LIMIT 1), '') as seq_status
		FROM contacts c
		ORDER BY last_time DESC`

	err := database.DB.Select(&contacts, query)
	if err != nil {
		log.Printf("Web: Error fetching contacts: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

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
		lastMsg := strings.ReplaceAll(c.LastMessage, "\n", " ")
		if len(lastMsg) > 30 {
			lastMsg = lastMsg[:27] + "..."
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
			statusIndicator = fmt.Sprintf(`<span class="w-2 h-2 rounded-full %s ml-2" title="Sequence: %s"></span>`, color, c.SeqStatus)
		}

		fmt.Fprintf(w, `
			<div class="p-3 border-b cursor-pointer hover:bg-blue-50 transition-colors contact-item group relative" 
			     hx-get="/messages/%d" 
				 hx-target="#chat-history"
				 hx-on:htmx:after-request="htmx.find('#chat-history').setAttribute('hx-get', '/messages/%d')"
				 onclick="document.querySelectorAll('.contact-item').forEach(el => el.classList.remove('bg-blue-100')); this.classList.add('bg-blue-100')">
				<div class="flex justify-between items-start">
					<div class="flex items-center">
						<p class="font-bold text-blue-800">%s %s</p>
						%s
					</div>
					<span class="text-[10px] text-gray-400">@%s</span>
				</div>
				<p class="text-xs text-gray-600 truncate">%s</p>
				
				<!-- Action menu button (dots) -->
				<button class="absolute right-2 bottom-2 opacity-0 group-hover:opacity-100 p-1 hover:bg-gray-200 rounded transition-opacity"
				        hx-get="/contacts/%d/actions"
						hx-target="#modal-container"
						hx-swap="innerHTML"
						onclick="event.stopPropagation()">
					<svg class="w-4 h-4 text-gray-500" fill="currentColor" viewBox="0 0 20 20">
						<path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z"></path>
					</svg>
				</button>
			</div>
		`, c.ID, c.ID, c.FirstName, c.LastName, statusIndicator, c.Username, html.EscapeString(lastMsg), c.ID)
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
					<p class="text-sm text-gray-800 whitespace-pre-wrap">%s</p>
					<p class="text-[9px] text-gray-500 mt-1 text-right">%s</p>
				</div>
			</div>
		`, align, bgColor, html.EscapeString(m.Text), m.Timestamp.Format("Jan 02, 15:04"))
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

	var allAccounts []models.Account
	database.DB.Select(&allAccounts, "SELECT * FROM accounts ORDER BY phone_number")

	var sequences []models.Sequence
	err := database.DB.Select(&sequences, "SELECT * FROM sequences ORDER BY created_at DESC")
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
	err = tx.Get(&foundID, "SELECT account_id FROM messages WHERE contact_id = ? AND account_id IS NOT NULL LIMIT 1", contactID)
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

	var maxOrder int
	database.DB.Get(&maxOrder, "SELECT COALESCE(max(order_index), 0) FROM interview_stages WHERE sequence_id = ?", seqID)

	_, err := database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, is_completed, order_index) VALUES (?, ?, 1, ?)",
		seqID, name, maxOrder+1)

	if err == nil {
		logger.Debug(logger.History, "Manually added stage '%s' to sequence %s", name, seqID)
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
