package web

import (
	"html/template"
	"net/http"
)

func (h *Handler) handleAccounts(w http.ResponseWriter, r *http.Request) {
	data, err := h.accRepo.GetAllWithIntegrations(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/accounts.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", data)
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
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleToggleAccount(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	err := h.accService.ToggleAccount(r.Context(), id, h.rootCtx)
	if err != nil {
		http.Error(w, err.Error(), 500)
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
	http.Redirect(w, r, "/accounts", 303)
}
