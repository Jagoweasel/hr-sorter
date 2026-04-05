package web

import (
	"net/http"
	"strings"
	"time"
)

func (h *Handler) handleContacts(w http.ResponseWriter, r *http.Request) {
	activeAccountID := r.URL.Query().Get("account_id")
	platformFilter := r.URL.Query().Get("platform")
	showDeclines := r.URL.Query().Get("show_declines") == "true"
	hideScreened := r.URL.Query().Get("hide_screened") == "true"
	hideUnanswered := r.URL.Query().Get("hide_unanswered") == "true"
	showIgnored := r.URL.Query().Get("show_ignored") == "true"
	sequenceFilter := r.URL.Query().Get("sequence_filter")

	filteredContacts, err := h.conService.GetFilteredContacts(r.Context(), activeAccountID, platformFilter, showDeclines, hideScreened, hideUnanswered, showIgnored, sequenceFilter)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.templates.RenderWithStatus(w, "fragments/contact_list.html", http.StatusOK, filteredContacts, h.getLocale(r))
}

func (h *Handler) handleIgnoreContact(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// Expected: /contacts/{id}/ignore or /contacts/{id}/restore
	action := "ignore"
	if strings.HasSuffix(path, "/restore") {
		action = "restore"
	}

	id := strings.TrimPrefix(path, "/contacts/")
	id = strings.TrimSuffix(id, "/"+action)

	ignored := action == "ignore"
	if err := h.conService.UpdateIgnored(r.Context(), id, ignored); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	// Trigger refresh of contact list
	w.Header().Set("HX-Trigger", "refreshContacts")
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleContactActions(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/ignore") || strings.HasSuffix(path, "/restore") {
		h.handleIgnoreContact(w, r)
		return
	}

	if strings.HasSuffix(path, "/actions") {
		id := strings.TrimPrefix(strings.TrimSuffix(path, "/actions"), "/contacts/")
		contact, _ := h.conRepo.GetByID(r.Context(), id)
		h.templates.RenderWithStatus(w, "fragments/modals/actions.html", http.StatusOK, contact, h.getLocale(r))
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

		h.templates.RenderWithStatus(w, "fragments/modals/create_sequence.html", http.StatusOK, struct {
			ContactID    string
			CompanyName  string
			VacancyName  string
			FirstMsgDate string
		}{
			ContactID:    contactID,
			CompanyName:  companyName,
			VacancyName:  vacancyName,
			FirstMsgDate: firstMsgDate,
		}, h.getLocale(r))
		return
	}

	if strings.HasSuffix(path, "/add-to-sequence-modal") {
		contactID := strings.TrimPrefix(strings.TrimSuffix(path, "/add-to-sequence-modal"), "/contacts/")
		sequences, _ := h.seqRepo.GetAll(r.Context(), "")

		h.templates.RenderWithStatus(w, "fragments/modals/add_to_sequence.html", http.StatusOK, struct {
			ContactID string
			Sequences interface{}
		}{
			ContactID: contactID,
			Sequences: sequences,
		}, h.getLocale(r))
		return
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
