package web

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
	"unicode"
)

func (h *Handler) handleContacts(w http.ResponseWriter, r *http.Request) {
	activeAccountID := r.URL.Query().Get("account_id")
	platformFilter := r.URL.Query().Get("platform")
	showDeclines := r.URL.Query().Get("show_declines") == "true"
	hideScreened := r.URL.Query().Get("hide_screened") == "true"
	hideUnanswered := r.URL.Query().Get("hide_unanswered") == "true"

	allContacts, err := h.conRepo.GetAll(r.Context(), activeAccountID, platformFilter, showDeclines)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	var activePatterns []string
	if hideScreened {
		patterns, _ := h.fltRepo.GetActivePatterns(r.Context())
		for _, p := range patterns {
			activePatterns = append(activePatterns, normalize(p))
		}
	}

	for _, c := range allContacts {
		if c.Platform == "hh" {
			if hideUnanswered {
				if c.MsgCount == 0 || !c.LastIsIncoming {
					continue
				}
			}

			if hideScreened && len(activePatterns) > 0 {
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
		}

		lastMsg := strings.ReplaceAll(c.LastMessage, "\n", " ")
		if lastMsg == "" {
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
			nameDisplay = deref(c.FirstName)
			subDisplay = deref(c.LastName)
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

func (h *Handler) handleContactActions(w http.ResponseWriter, r *http.Request) {
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
		contact, _ := h.conRepo.GetByID(r.Context(), contactID)

		companyName := ""
		vacancyName := "Senior Go Dev"
		if contact.Platform == "hh" {
			companyName = deref(contact.FirstName)
			vacancyName = deref(contact.LastName)
		}

		firstMsgDate := time.Now().Format("2006-01-02T15:04")

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
									<input type="text" name="company_name" value="%s" required class="block w-full border-2 border-gray-100 rounded-xl p-3 focus:border-blue-500 focus:ring-0 transition-colors text-sm font-bold bg-gray-50" placeholder="Google">
								</div>
								<div>
									<label class="block text-[10px] font-black text-gray-400 uppercase tracking-[0.2em] mb-2">Vacancy</label>
									<input type="text" name="vacancy_name" value="%s" required class="block w-full border-2 border-gray-100 rounded-xl p-3 focus:border-blue-500 focus:ring-0 transition-colors text-sm font-bold bg-gray-50" placeholder="Senior Go Dev">
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
		`, contactID, html.EscapeString(companyName), html.EscapeString(vacancyName), firstMsgDate)
		return
	}

	if strings.HasSuffix(path, "/add-to-sequence-modal") {
		contactID := strings.TrimPrefix(strings.TrimSuffix(path, "/add-to-sequence-modal"), "/contacts/")
		sequences, _ := h.seqRepo.GetAll(r.Context(), "")

		options := ""
		for _, s := range sequences {
			if s.Status != "accepted" && s.Status != "rejected" {
				options += fmt.Sprintf(`<option value="%d">%s — %s</option>`, s.ID, s.CompanyName, s.VacancyName)
			}
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

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func normalize(s string) string {
	f := func(r rune) bool { return unicode.IsSpace(r) }
	words := strings.FieldsFunc(s, f)
	return strings.ToLower(strings.Join(words, " "))
}
