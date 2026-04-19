package web

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
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

	// Use background context with timeout to avoid "context canceled" if user closes the modal/page too fast
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := h.seqService.CreateSequence(ctx, company, vacancy, contactID, initialDateStr); err != nil {
		log.Printf("[Web] Error creating sequence: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
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

	// Use background context for bulk operation as well
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := h.seqService.BulkCreateSequences(ctx, accountID, platform, showDeclines, hideScreened, hideUnanswered); err != nil {
		log.Printf("[Web] Error bulk creating sequences: %v", err)
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
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	stageID := r.URL.Query().Get("id")
	completed := r.URL.Query().Get("completed") == "true"
	view := r.URL.Query().Get("view")

	if err := h.seqService.UpdateStageCompletion(r.Context(), stageID, completed); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if view != "" {
		// Partial update for a specific card
		// We need sequence ID, but stage completion update might need us to fetch the sequence
		stage, _ := h.seqRepo.GetStageByID(r.Context(), stageID)
		if stage != nil {
			details, _ := h.seqRepo.GetFullDetailsByID(r.Context(), stage.SequenceID)
			if details != nil {
				details.ColumnDefs = h.getColumnDefs(r)
				h.templates.RenderWithStatus(w, r, "fragments/pipeline/card_"+view+".html", http.StatusOK, details, h.getLocale(r))
				return
			}
		}
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	seqIDStr := r.URL.Query().Get("sequence_id")
	category := r.URL.Query().Get("category")
	name := r.URL.Query().Get("name")
	view := r.URL.Query().Get("view")
	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

	if err := h.seqService.AddStage(r.Context(), seqID, category, name); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if view == "timeline" {
		details, _ := h.seqRepo.GetFullDetailsByID(r.Context(), seqID)
		if details != nil {
			details.ColumnDefs = h.getColumnDefs(r)
			w.Header().Set("HX-Trigger", "closeModal")
			h.templates.RenderWithStatus(w, r, "fragments/pipeline/card_timeline.html", http.StatusOK, details, h.getLocale(r))
			return
		}
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleMoveSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	seqIDStr := r.URL.Query().Get("id")
	status := r.URL.Query().Get("status")
	view := r.URL.Query().Get("view")
	if status == "" {
		status = r.FormValue("status")
	}
	seqID, _ := strconv.ParseInt(seqIDStr, 10, 64)

	if status == "" {
		log.Printf("[Web] Warning: handleMoveSequence called with empty status for seq %s", seqIDStr)
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}

	if err := h.seqService.MoveSequence(r.Context(), seqID, status); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if view == "kanban" {
		h.handlePipeline(w, r)
		return
	}

	if view == "timeline" {
		details, _ := h.seqRepo.GetFullDetailsByID(r.Context(), seqID)
		if details != nil {
			details.ColumnDefs = h.getColumnDefs(r)
			h.templates.RenderWithStatus(w, r, "fragments/pipeline/card_"+view+".html", http.StatusOK, details, h.getLocale(r))
			return
		}
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleEditSequenceModal(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	seq, err := h.seqRepo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.templates.RenderWithStatus(w, r, "fragments/modals/edit_sequence.html", http.StatusOK, seq, h.getLocale(r))
}

func (h *Handler) handleUpdateSequence(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	company := r.FormValue("company_name")
	vacancy := r.FormValue("vacancy_name")
	link := r.FormValue("vacancy_link")
	reason := r.FormValue("rejection_reason")
	comment := r.FormValue("summary_comment")

	if err := h.seqRepo.UpdateDetails(r.Context(), id, company, vacancy, link, reason, comment); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleDeleteSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	seqID := r.URL.Query().Get("id")
	if err := h.seqRepo.Delete(r.Context(), seqID); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		// If it's an HTMX request, we can just return 200 and tell HTMX to delete the element
		// if the target was 'closest .group/card' or similar.
		// However, to be safe and support different targets, we can use HX-Reswap
		w.Header().Set("HX-Reswap", "delete")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleBulkDeleteSequences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	ids := r.Form["sequence_ids"]
	for _, id := range ids {
		_ = h.seqRepo.Delete(r.Context(), id)
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
}

func (h *Handler) handleAddStageModal(w http.ResponseWriter, r *http.Request) {
	seqID := r.URL.Query().Get("sequence_id")
	view := r.URL.Query().Get("view")
	h.templates.RenderWithStatus(w, r, "fragments/modals/add_stage.html", http.StatusOK, map[string]string{
		"SequenceID": seqID,
		"View":       view,
	}, h.getLocale(r))
}

func (h *Handler) getRedirectURL(r *http.Request) string {
	referer := r.Header.Get("Referer")
	if referer == "" {
		return "/"
	}

	// Parse referer to get path and query
	if strings.Contains(referer, "://") {
		parts := strings.SplitN(referer, "//", 2)
		if len(parts) > 1 {
			subParts := strings.SplitN(parts[1], "/", 2)
			if len(subParts) > 1 {
				return "/" + subParts[1]
			}
		}
	} else if strings.HasPrefix(referer, "/") {
		return referer
	}

	if strings.Contains(referer, "view=timeline") {
		return "/pipeline?view=timeline"
	}
	if strings.Contains(referer, "pipeline") {
		return "/pipeline"
	}
	return "/"
}
