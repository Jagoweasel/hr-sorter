package web

import (
	"fmt"
	"hr-sorter/internal/logger"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (h *Handler) handleCreateSequence(w http.ResponseWriter, r *http.Request) {
	logger.Debug(logger.AddSequence, "handleCreateSequence triggered")
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	company := r.FormValue("company_name")
	vacancy := r.FormValue("vacancy_name")
	contactID := r.FormValue("contact_id")
	initialDateStr := r.FormValue("initial_date")

	logger.Debug(logger.AddSequence, "Creating sequence for Company='%s', Vacancy='%s', ContactID='%s'", company, vacancy, contactID)

	tx, err := h.seqRepo.BeginTx(r.Context())
	if err != nil {
		logger.Debug(logger.AddSequence, "Error starting transaction: %v", err)
		http.Error(w, "Database error", 500)
		return
	}
	defer tx.Rollback()

	// Find the account associated with this contact
	var accountID *int64
	if contactID != "" {
		accID, err := h.conRepo.GetAccountIDByContactID(r.Context(), contactID)
		if err == nil {
			accountID = accID
			logger.Debug(logger.AddSequence, "Found account ID %d from contact", *accID)
		}
	}

	if accountID == nil {
		activeAccounts, err := h.accRepo.GetActive(r.Context())
		if err == nil && len(activeAccounts) > 0 {
			id := activeAccounts[0].ID
			accountID = &id
			logger.Debug(logger.AddSequence, "Using first active account ID %d", id)
		} else {
			allAccounts, err := h.accRepo.GetAll(r.Context())
			if err == nil && len(allAccounts) > 0 {
				id := allAccounts[0].ID
				accountID = &id
				logger.Debug(logger.AddSequence, "Fallback to any account ID %d", id)
			}
		}
	}

	seqID, err := h.seqRepo.Create(r.Context(), tx, accountID, company, vacancy, "initial")
	if err != nil {
		logger.Debug(logger.AddSequence, "Error inserting sequence: %v", err)
		http.Error(w, "Database error", 500)
		return
	}
	logger.Debug(logger.AddSequence, "Sequence created with ID %d", seqID)

	if contactID != "" {
		err = h.seqRepo.LinkContact(r.Context(), tx, seqID, contactID)
		if err != nil {
			logger.Debug(logger.AddSequence, "Error linking contact: %v", err)
		}
	}

	initialDate, _ := time.Parse("2006-01-02T15:04", initialDateStr)
	h.seqRepo.CreateStage(r.Context(), tx, seqID, "Initial Contact", initialDate, true, 0)
	h.seqRepo.CreateStage(r.Context(), tx, seqID, "HR Screening", nil, false, 1)
	h.seqRepo.CreateStage(r.Context(), tx, seqID, "Final Interview", nil, false, 2)
	h.seqRepo.CreateStage(r.Context(), tx, seqID, "Offer", nil, false, 3)

	if err := tx.Commit(); err != nil {
		logger.Debug(logger.AddSequence, "Error committing transaction: %v", err)
		http.Error(w, "Commit failed", 500)
		return
	}
	logger.Debug(logger.AddSequence, "Transaction committed successfully for sequence %d", seqID)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddToSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.FormValue("sequence_id")
	contactID := r.FormValue("contact_id")
	h.seqRepo.LinkContact(r.Context(), nil, seqID, contactID)

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Redirect", h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	stageID := r.URL.Query().Get("id")
	completed := r.URL.Query().Get("completed") == "true"

	stage, err := h.seqRepo.GetStageByID(r.Context(), stageID)
	if err != nil {
		http.Error(w, "Stage not found", 404)
		return
	}
	h.seqRepo.UpdateStageStatus(r.Context(), stageID, completed)

	// Auto status update logic
	last, _ := h.seqRepo.GetLastCompletedStage(r.Context(), stage.SequenceID)
	if last != nil {
		name := strings.ToLower(last.Name)
		newStatus := "initial"
		if strings.Contains(name, "offer") {
			newStatus = "offer"
		} else if strings.Contains(name, "final") {
			newStatus = "final"
		} else if strings.Contains(name, "tech") {
			newStatus = "tech"
		} else if strings.Contains(name, "screen") {
			newStatus = "screening"
		}
		h.seqRepo.UpdateStatus(r.Context(), stage.SequenceID, newStatus)
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStage(w http.ResponseWriter, r *http.Request) {
	seqIDStr := r.URL.Query().Get("sequence_id")
	category := r.URL.Query().Get("category") // screening, tech, final, offer
	name := r.URL.Query().Get("name")

	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

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

		count, _ := h.seqRepo.GetStageCountByCategory(r.Context(), seqID, label, category)
		name = fmt.Sprintf("%s %d", strings.Title(label), count+1)
	}

	// Determine insertion point based on hierarchy
	stages, _ := h.seqRepo.GetStages(r.Context(), seqID)

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
	h.seqRepo.ShiftStages(r.Context(), seqID, insertAt)

	err := h.seqRepo.CreateStage(r.Context(), nil, seqID, name, nil, true, insertAt)

	if err == nil {
		logger.Debug(logger.History, "Manually added stage '%s' to sequence %d at index %d", name, seqID, insertAt)
		// Update sequence status
		status := category
		if category == "initial" {
			status = "initial"
		}
		h.seqRepo.UpdateStatus(r.Context(), seqID, status)
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleMoveSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	h.seqRepo.UpdateStatus(r.Context(), seqID, status)

	if status == "rejected" {
		incomplete, _ := h.seqRepo.GetFirstIncompleteStage(r.Context(), seqID)
		if incomplete != nil {
			h.seqRepo.UpdateStageStatus(r.Context(), incomplete.ID, true)
		}
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	h.seqRepo.Delete(r.Context(), seqID)
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStageModal(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) getRedirectURL(r *http.Request) string {
	referer := r.Header.Get("Referer")
	if referer == "" {
		return "/"
	}
	if strings.Contains(referer, "view=timeline") {
		return "/pipeline?view=timeline"
	}
	if strings.Contains(referer, "pipeline") {
		return "/pipeline"
	}
	return "/"
}
