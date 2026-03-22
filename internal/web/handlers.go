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
	mux.HandleFunc("/stages/update", handleUpdateStage)
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
				 hx-on::after-request="htmx.find('#chat-history').setAttribute('hx-get', '/messages/%d')"
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
						hx-target="next .actions-menu"
						hx-swap="innerHTML"
						onclick="event.stopPropagation()">
					<svg class="w-4 h-4 text-gray-500" fill="currentColor" viewBox="0 0 20 20">
						<path d="M10 6a2 2 0 110-4 2 2 0 010 4zM10 12a2 2 0 110-4 2 2 0 010 4zM10 18a2 2 0 110-4 2 2 0 010 4z"></path>
					</svg>
				</button>
				<div class="actions-menu absolute right-8 bottom-2 z-50"></div>
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
}

type PipelineColumn struct {
	ID          string
	Label       string
	ColorClass  string
	BorderClass string
	Sequences   []SequenceWithDetails
}

func handlePipeline(w http.ResponseWriter, r *http.Request) {
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
		database.DB.Select(&recruiters, `
			SELECT c.* FROM contacts c 
			JOIN sequence_contacts sc ON c.id = sc.contact_id 
			WHERE sc.sequence_id = ?`, s.ID)

		var stages []models.InterviewStage
		database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? ORDER BY order_index ASC", s.ID)

		detailedSeqs = append(detailedSeqs, SequenceWithDetails{
			Sequence:   s,
			Recruiters: recruiters,
			Stages:     stages,
		})
	}

	columns := []PipelineColumn{
		{ID: "initial", Label: "Initial", ColorClass: "bg-blue-50", BorderClass: "border-blue-200"},
		{ID: "screening", Label: "Screening", ColorClass: "bg-indigo-50", BorderClass: "border-indigo-200"},
		{ID: "tech", Label: "Technical", ColorClass: "bg-purple-50", BorderClass: "border-purple-200"},
		{ID: "final", Label: "Final Interview", ColorClass: "bg-pink-50", BorderClass: "border-pink-200"},
		{ID: "offer", Label: "Offer", ColorClass: "bg-yellow-50", BorderClass: "border-yellow-200"},
		{ID: "accepted", Label: "Accepted", ColorClass: "bg-green-50", BorderClass: "border-green-200"},
		{ID: "rejected", Label: "Rejected", ColorClass: "bg-red-50", BorderClass: "border-red-200"},
	}

	// Distribute sequences into columns
	for i := range columns {
		for _, s := range detailedSeqs {
			if s.Status == columns[i].ID {
				columns[i].Sequences = append(columns[i].Sequences, s)
			}
		}
	}

	data := struct {
		Columns []PipelineColumn
	}{
		Columns: columns,
	}

	tmpl := template.New("layout.html").Funcs(template.FuncMap{
		"add": func(a, b int) int {
			return a + b
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
			<div class="bg-white border shadow-xl rounded-lg py-2 w-48 text-sm" hx-on::click-outside="this.remove()">
				<button class="w-full text-left px-4 py-2 hover:bg-blue-50 text-blue-700 font-medium"
				        hx-get="/contacts/%s/create-sequence-modal"
						hx-target="#modal-container"
						onclick="this.parentElement.remove()">
					Start Sequence
				</button>
				<button class="w-full text-left px-4 py-2 hover:bg-blue-50 text-gray-700"
				        hx-get="/contacts/%s/add-to-sequence-modal"
						hx-target="#modal-container"
						onclick="this.parentElement.remove()">
					Add to Existing
				</button>
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
			<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[100]" id="modal">
				<div class="bg-white rounded-xl shadow-2xl p-6 w-full max-w-lg overflow-y-auto max-h-[90vh]" hx-on::click-outside="htmx.remove('#modal')">
					<h2 class="text-xl font-bold mb-4 border-b pb-2">Start New Interview Sequence</h2>
					<form hx-post="/sequences/create" hx-target="body">
						<input type="hidden" name="contact_id" value="%s">
						<div class="space-y-4">
							<div class="grid grid-cols-2 gap-4">
								<div>
									<label class="block text-xs font-bold text-gray-500 uppercase">Company Name</label>
									<input type="text" name="company_name" required class="mt-1 block w-full border rounded-md p-2 bg-gray-50 focus:bg-white" placeholder="Google">
								</div>
								<div>
									<label class="block text-xs font-bold text-gray-500 uppercase">Vacancy Name</label>
									<input type="text" name="vacancy_name" required class="mt-1 block w-full border rounded-md p-2 bg-gray-50 focus:bg-white" placeholder="Senior Go Developer">
								</div>
							</div>
							<div>
								<label class="block text-xs font-bold text-gray-500 uppercase">Initial Contact Date</label>
								<input type="datetime-local" name="initial_date" value="%s" class="mt-1 block w-full border rounded-md p-2 bg-gray-50">
							</div>
							
							<div class="border-t pt-4">
								<p class="text-xs font-bold text-gray-500 uppercase mb-2">Technical Stages</p>
								<div id="tech-stages" class="space-y-2">
									<div class="flex items-center space-x-2">
										<input type="text" name="tech_stage_name[]" value="Technical Interview 1" class="flex-1 border rounded-md p-2 text-sm">
										<select name="tech_stage_type[]" class="border rounded-md p-2 text-sm bg-gray-50">
											<option value="">General</option>
											<option value="Theory">Theory</option>
											<option value="Live Coding">Live Coding</option>
											<option value="System Design">System Design</option>
										</select>
									</div>
								</div>
								<button type="button" 
								        onclick="const div = document.createElement('div'); div.className = 'flex items-center space-x-2 mt-2'; div.innerHTML = '<input type=\'text\' name=\'tech_stage_name[]\' value=\'Technical Interview ' + (document.querySelectorAll('#tech-stages > div').length + 1) + '\' class=\'flex-1 border rounded-md p-2 text-sm\'><select name=\'tech_stage_type[]\' class=\'border rounded-md p-2 text-sm bg-gray-50\'><option value=\'\'>General</option><option value=\'Theory\'>Theory</option><option value=\'Live Coding\'>Live Coding</option><option value=\'System Design\'>System Design</option></select>'; document.getElementById('tech-stages').appendChild(div)"
										class="mt-2 text-xs text-blue-600 hover:text-blue-800 font-bold">+ Add Tech Stage</button>
							</div>
						</div>
						<div class="mt-8 flex justify-end space-x-3 border-t pt-4">
							<button type="button" onclick="htmx.remove('#modal')" class="px-4 py-2 text-gray-600 hover:text-gray-800 text-sm font-medium">Cancel</button>
							<button type="submit" class="px-6 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700 font-bold shadow-lg">Create Sequence</button>
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
			options += fmt.Sprintf(`<option value="%d">%s - %s</option>`, s.ID, s.CompanyName, s.VacancyName)
		}

		fmt.Fprintf(w, `
			<div class="fixed inset-0 bg-black bg-opacity-50 flex items-center justify-center z-[100]" id="modal">
				<div class="bg-white rounded-xl shadow-2xl p-6 w-full max-w-md" hx-on::click-outside="htmx.remove('#modal')">
					<h2 class="text-xl font-bold mb-4">Add Recruiter to Sequence</h2>
					<form hx-post="/sequences/add-contact" hx-target="body">
						<input type="hidden" name="contact_id" value="%s">
						<div class="space-y-4">
							<div>
								<label class="block text-sm font-medium text-gray-700">Select Sequence</label>
								<select name="sequence_id" required class="mt-1 block w-full border rounded-md p-2">
									%s
								</select>
							</div>
						</div>
						<div class="mt-6 flex justify-end space-x-3">
							<button type="button" onclick="htmx.remove('#modal')" class="px-4 py-2 text-gray-600 hover:text-gray-800">Cancel</button>
							<button type="submit" class="px-4 py-2 bg-blue-600 text-white rounded-md hover:bg-blue-700">Add to Sequence</button>
						</div>
					</form>
				</div>
			</div>
		`, contactID, options)
		return
	}
}

func handleCreateSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	company := r.FormValue("company_name")
	vacancy := r.FormValue("vacancy_name")
	contactID := r.FormValue("contact_id")
	initialDateStr := r.FormValue("initial_date")
	techNames := r.Form["tech_stage_name[]"]
	techTypes := r.Form["tech_stage_type[]"]

	// Find the account associated with this contact
	var accountID int64
	err = database.DB.Get(&accountID, "SELECT account_id FROM messages WHERE contact_id = ? LIMIT 1", contactID)
	if err != nil {
		log.Printf("Pipeline: Warning - could not find associated account for contact %s, using first active account", contactID)
		database.DB.Get(&accountID, "SELECT id FROM accounts WHERE status = 'active' LIMIT 1")
	}

	res, err := database.DB.Exec("INSERT INTO sequences (account_id, company_name, vacancy_name, status) VALUES (?, ?, ?, ?)", accountID, company, vacancy, "initial")
	if err != nil {
		log.Printf("Pipeline: Error creating sequence: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	seqID, _ := res.LastInsertId()
	log.Printf("Pipeline: Created sequence ID %d for %s (%s)", seqID, company, vacancy)

	if contactID != "" {
		database.DB.Exec("INSERT INTO sequence_contacts (sequence_id, contact_id) VALUES (?, ?)", seqID, contactID)
		log.Printf("Pipeline: Linked recruiter ID %s to sequence ID %d", contactID, seqID)
	}

	// Create initial stage
	initialDate, _ := time.Parse("2006-01-02T15:04", initialDateStr)
	database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, scheduled_at, is_completed, order_index) VALUES (?, ?, ?, ?, ?)",
		seqID, "Initial Contact", initialDate, 1, 0)

	// Add HR Screening
	database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)",
		seqID, "HR Screening", 1)

	// Add Technical Stages
	currIdx := 2
	for i, name := range techNames {
		if name == "" {
			continue
		}
		tType := techTypes[i]
		database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, stage_type, order_index) VALUES (?, ?, ?, ?)",
			seqID, name, tType, currIdx)
		currIdx++
	}

	// Add default final stages
	database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)",
		seqID, "Final Interview", currIdx)
	database.DB.Exec("INSERT INTO interview_stages (sequence_id, name, order_index) VALUES (?, ?, ?)",
		seqID, "Offer", currIdx+1)

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
		log.Printf("Pipeline: Error adding contact %s to sequence %s: %v", contactID, seqID, err)
		http.Error(w, err.Error(), 500)
		return
	}

	log.Printf("Pipeline: Added recruiter ID %s to sequence ID %s", contactID, seqID)
	http.Redirect(w, r, "/", 303)
}

func handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	stageID := r.URL.Query().Get("id")
	completed := r.URL.Query().Get("completed") == "true"

	database.DB.Exec("UPDATE interview_stages SET is_completed = ? WHERE id = ?", completed, stageID)

	var stage models.InterviewStage
	database.DB.Get(&stage, "SELECT * FROM interview_stages WHERE id = ?", stageID)

	log.Printf("Pipeline: Updated stage ID %s to completed=%v", stageID, completed)

	// Update sequence status based on the last completed stage
	var stages []models.InterviewStage
	database.DB.Select(&stages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index DESC LIMIT 1", stage.SequenceID)

	newStatus := "initial"
	if len(stages) > 0 {
		s := stages[0]
		name := strings.ToLower(s.Name)
		if strings.Contains(name, "offer") {
			newStatus = "offer"
		} else if strings.Contains(name, "final") {
			newStatus = "final"
		} else if strings.Contains(name, "tech") {
			newStatus = "tech"
		} else if strings.Contains(name, "screen") {
			newStatus = "screening"
		}
	}

	database.DB.Exec("UPDATE sequences SET status = ? WHERE id = ?", newStatus, stage.SequenceID)
	log.Printf("Pipeline: Sequence ID %d status automatically updated to '%s'", stage.SequenceID, newStatus)

	http.Redirect(w, r, "/pipeline", 303)
}

func handleMoveSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")

	database.DB.Exec("UPDATE sequences SET status = ? WHERE id = ?", status, seqID)
	log.Printf("Pipeline: Sequence ID %s manually moved to status '%s'", seqID, status)
	http.Redirect(w, r, "/pipeline", 303)
}

func handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	database.DB.Exec("DELETE FROM sequences WHERE id = ?", seqID)
	log.Printf("Pipeline: Deleted sequence ID %s", seqID)
	http.Redirect(w, r, "/pipeline", 303)
}
