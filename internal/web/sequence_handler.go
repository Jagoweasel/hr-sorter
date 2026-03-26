package web

import (
	"net/http"
	"strconv"
	"strings"
)

func (h *Handler) handleCreateSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	company := r.FormValue("company_name")
	vacancy := r.FormValue("vacancy_name")
	contactID := r.FormValue("contact_id")
	initialDateStr := r.FormValue("initial_date")

	if _, err := h.seqService.CreateSequence(r.Context(), company, vacancy, contactID, initialDateStr); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleBulkAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", 405)
		return
	}

	accountID := r.FormValue("account_id")
	platform := r.FormValue("platform")
	showDeclines := r.FormValue("show_declines") == "true"
	hideScreened := r.FormValue("hide_screened") == "true"
	hideUnanswered := r.FormValue("hide_unanswered") == "true"

	if _, err := h.seqService.BulkCreateSequences(r.Context(), accountID, platform, showDeclines, hideScreened, hideUnanswered); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Header().Set("HX-Trigger", "refreshContacts")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleAddToSequence(w http.ResponseWriter, r *http.Request) {
	seqIDStr := r.FormValue("sequence_id")
	contactID := r.FormValue("contact_id")
	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

	if err := h.seqRepo.LinkContact(r.Context(), nil, seqID, contactID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	stageID := r.URL.Query().Get("id")
	completed := r.URL.Query().Get("completed") == "true"

	if err := h.seqService.UpdateStageCompletion(r.Context(), stageID, completed); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStage(w http.ResponseWriter, r *http.Request) {
	seqIDStr := r.URL.Query().Get("sequence_id")
	category := r.URL.Query().Get("category")
	name := r.URL.Query().Get("name")
	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

	if err := h.seqService.AddStage(r.Context(), seqID, category, name); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleMoveSequence(w http.ResponseWriter, r *http.Request) {
	seqIDStr := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

	if err := h.seqService.MoveSequence(r.Context(), seqID, status); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("id")
	if err := h.seqRepo.Delete(r.Context(), seqID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStageModal(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("sequence_id")
	h.templates.RenderWithStatus(w, "fragments/modals/add_stage.html", http.StatusOK, map[string]string{"SequenceID": seqID})
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
