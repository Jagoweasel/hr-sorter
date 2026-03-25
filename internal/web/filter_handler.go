package web

import (
	"net/http"
)

func (h *Handler) handleGetFilters(w http.ResponseWriter, r *http.Request) {
	filters, err := h.fltRepo.GetAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.templates.RenderWithStatus(w, "fragments/filter_list.html", http.StatusOK, filters)
}

func (h *Handler) handleAddFilter(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if err := h.fltService.AddFilter(r.Context(), pattern); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := h.fltService.DeleteFilter(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleToggleFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := h.fltService.ToggleFilter(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}
