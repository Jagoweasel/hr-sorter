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

	h.templates.RenderWithStatus(w, r, "accounts.html", http.StatusOK, data, h.getLocale(r))
}

func (h *Handler) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.FormValue("name")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	err := h.accService.CreateAccount(r.Context(), name)
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
	h.templates.RenderWithStatus(w, r, "fragments/modals/edit_account.html", http.StatusOK, acc, h.getLocale(r))
}

func (h *Handler) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.FormValue("id")
	name := r.FormValue("name")
	slug := r.FormValue("slug")
	if name == "" {
		http.Error(w, "Name required", 400)
		return
	}
	err := h.accService.UpdateAccount(r.Context(), id, name, slug)
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
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
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
