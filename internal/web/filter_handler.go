package web

import (
	"io"
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

func (h *Handler) handleExportFilters(w http.ResponseWriter, r *http.Request) {
	if err := h.fltService.ExportFilters(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleImportFilters(w http.ResponseWriter, r *http.Request) {
	if err := h.fltService.ImportFilters(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleUploadFilters(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB limit
		http.Error(w, "Unable to parse form", 400)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "File is required", 400)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Unable to read file", 500)
		return
	}

	if err := h.fltService.ImportFromJSON(r.Context(), data); err != nil {
		http.Error(w, "Invalid JSON format (list of strings required)", 400)
		return
	}

	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleClearFilters(w http.ResponseWriter, r *http.Request) {
	if err := h.fltService.ClearFilters(r.Context()); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Trigger", "refreshFilters")
	w.WriteHeader(http.StatusNoContent)
}
