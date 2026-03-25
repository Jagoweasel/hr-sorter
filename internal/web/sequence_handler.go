package web

import (
	"fmt"
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
		w.Header().Set("HX-Redirect", h.getRedirectURL(r))
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, h.getRedirectURL(r), 303)
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
		w.Header().Set("HX-Redirect", h.getRedirectURL(r))
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
