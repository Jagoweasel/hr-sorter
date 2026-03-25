package web

import (
	"fmt"
	"html"
	"net/http"
)

func (h *Handler) handleGetFilters(w http.ResponseWriter, r *http.Request) {
	filters, err := h.fltRepo.GetAll(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	w.Write([]byte(`<div class="space-y-3">`))
	for _, f := range filters {
		activeClass := "bg-green-100 text-green-700"
		if !f.IsActive {
			activeClass = "bg-gray-100 text-gray-500"
		}
		fmt.Fprintf(w, `
			<div class="flex items-center justify-between p-3 bg-gray-50 rounded-xl border border-gray-100 group">
				<div class="flex items-center space-x-3">
					<button hx-post="/filters/toggle?id=%d" hx-target="#filter-list-content" class="px-2 py-0.5 rounded text-[8px] font-black uppercase tracking-widest %s">
						%s
					</button>
					<span class="text-sm font-bold text-gray-700">%s</span>
				</div>
				<button hx-post="/filters/delete?id=%d" hx-target="#filter-list-content" hx-confirm="Delete pattern?" class="opacity-0 group-hover:opacity-100 p-1 text-red-400 hover:bg-red-50 rounded-lg transition-all">
					<svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"></path></svg>
				</button>
			</div>
		`, f.ID, activeClass, map[bool]string{true: "Active", false: "Off"}[f.IsActive], html.EscapeString(f.Pattern), f.ID)
	}
	if len(filters) == 0 {
		w.Write([]byte(`<p class="text-center text-gray-400 italic text-sm py-4">No filters defined yet</p>`))
	}
	w.Write([]byte(`</div>`))
}

func (h *Handler) handleAddFilter(w http.ResponseWriter, r *http.Request) {
	pattern := r.FormValue("pattern")
	if err := h.fltService.AddFilter(r.Context(), pattern); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.handleGetFilters(w, r)
}

func (h *Handler) handleDeleteFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := h.fltService.DeleteFilter(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.handleGetFilters(w, r)
}

func (h *Handler) handleToggleFilter(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := h.fltService.ToggleFilter(r.Context(), id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.handleGetFilters(w, r)
}
