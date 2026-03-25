package web

import (
	"fmt"
	"hr-sorter/internal/hhclient"
	"net/http"
	"strconv"
)

func (h *Handler) handleCreateIntegration(w http.ResponseWriter, r *http.Request) {
	accID := r.FormValue("account_id")
	platform := r.FormValue("platform")
	identifier := r.FormValue("identifier")
	apiIDStr := r.FormValue("api_id")
	apiHash := r.FormValue("api_hash")

	apiID := 0
	if apiIDStr != "" {
		apiID, _ = strconv.Atoi(apiIDStr)
	}

	if err := h.intService.CreateIntegration(r.Context(), accID, platform, identifier, apiID, apiHash, h.rootCtx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if err := h.intService.ToggleIntegration(r.Context(), id, h.rootCtx); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if err := h.intService.DeleteIntegration(r.Context(), idStr); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	integration, err := h.intRepo.GetByID(r.Context(), idStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "error"}`))
		return
	}

	authURL := ""
	if integration.Platform == "hh" && integration.Status == "pending_auth" {
		authURL = hhclient.GetAuthorizeURL()
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "%s", "identifier": "%s", "platform": "%s", "auth_url": "%s"}`,
		integration.Status, integration.Identifier, integration.Platform, authURL)
}

func (h *Handler) handleSubmitCode(w http.ResponseWriter, r *http.Request) {
	idStr := r.FormValue("integration_id")
	code := r.FormValue("code")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	ok := h.tgManager.SubmitCode(id, code)
	if ok {
		w.Write([]byte(`{"ok": true}`))
	} else {
		w.Write([]byte(`{"ok": false, "error": "no pending auth request"}`))
	}
}

func (h *Handler) handleSubmitHHCode(w http.ResponseWriter, r *http.Request) {
	idStr := r.FormValue("integration_id")
	code := r.FormValue("code")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	if err := h.intService.HandleHHAuth(r.Context(), id, code, h.rootCtx); err != nil {
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "%v"}`, err)))
		return
	}

	w.Write([]byte(`{"ok": true}`))
}

func (h *Handler) handleSubmitPassword(w http.ResponseWriter, r *http.Request) {
	idStr := r.FormValue("integration_id")
	password := r.FormValue("password")
	id, _ := strconv.ParseInt(idStr, 10, 64)

	ok := h.tgManager.SubmitPassword(id, password)
	if ok {
		w.Write([]byte(`{"ok": true}`))
	} else {
		w.Write([]byte(`{"ok": false, "error": "no pending auth request"}`))
	}
}
