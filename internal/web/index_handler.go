package web

import (
	"html/template"
	"net/http"
)

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	accounts, err := h.accRepo.GetActive(r.Context())
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	tmpl := template.Must(template.ParseFiles(
		"templates/layout.html",
		"templates/index.html",
	))
	tmpl.ExecuteTemplate(w, "layout.html", accounts)
}
