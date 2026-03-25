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

	filteredContacts, err := h.conService.GetFilteredContacts(r.Context(), activeAccountID, platformFilter, showDeclines, hideScreened, hideUnanswered)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.templates.RenderWithStatus(w, "fragments/contact_list.html", http.StatusOK, filteredContacts)
}

func (h *Handler) handleContactActions(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/actions") {
		id := strings.TrimPrefix(strings.TrimSuffix(path, "/actions"), "/contacts/")
		h.templates.RenderWithStatus(w, "fragments/modals/actions.html", http.StatusOK, map[string]string{"ID": id})
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
		})
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
		})
		return
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
