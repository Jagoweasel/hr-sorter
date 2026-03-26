package web

import (
	"net/http"
)

func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	data, err := h.accRepo.GetAllWithIntegrations(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	h.templates.RenderWithStatus(w, "accounts.html", http.StatusOK, data)
}

func (h *Handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	err := h.accRepo.Create(r.Context(), name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", "/accounts")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleEditAccountModal(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	acc, err := h.accRepo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.templates.RenderWithStatus(w, "fragments/modals/edit_account.html", http.StatusOK, acc)
}

func (h *Handler) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.FormValue("id")
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	err := h.accRepo.UpdateName(r.Context(), id, name)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", "/accounts")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleToggleAccount(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	err := h.accService.ToggleAccount(r.Context(), id, h.rootCtx)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", "/accounts")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	err := h.accService.DeleteAccount(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", "/accounts")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}
