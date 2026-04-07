package web

import (
	"hr-sorter/internal/models"
	"net/http"
	"strconv"
)

func (h *Handler) handleMapping(w http.ResponseWriter, r *http.Request) {
	rules, err := h.mapRepo.GetAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.templates.RenderWithStatus(w, r, "mapping.html", http.StatusOK, rules, h.getLocale(r))
}

func (h *Handler) handleSaveMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	pattern := r.FormValue("pattern")
	category := r.FormValue("category")

	rule := &models.MappingRule{
		Pattern:  pattern,
		Category: category,
	}
	if idStr != "" {
		id, _ := strconv.ParseInt(idStr, 10, 64)
		rule.ID = id
	}

	if err := h.mapRepo.Save(r.Context(), rule); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, "/mapping")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/mapping", http.StatusSeeOther)
	}
}

func (h *Handler) handleDeleteMapping(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.URL.Query().Get("id")
	if err := h.mapRepo.Delete(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if r.Header.Get("HX-Request") != "" {
		h.setHXLocation(w, "/mapping")
		w.WriteHeader(http.StatusOK)
	} else {
		http.Redirect(w, r, "/mapping", http.StatusSeeOther)
	}
}
