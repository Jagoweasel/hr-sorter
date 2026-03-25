package web

import (
	"fmt"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"net/http"
	"strconv"
	"time"
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

	status := "pending_auth"
	sessionPath := ""
	var userAgent *string
	if platform == "tg" {
		sessionDir := "sessions"
		sessionPath = fmt.Sprintf("%s/%s.json", sessionDir, identifier)
	} else if platform == "hh" {
		ua := hhclient.GenerateAndroidUserAgent()
		userAgent = &ua
	}

	id, err := h.intRepo.Create(r.Context(), accID, platform, identifier, apiID, apiHash, status, sessionPath, userAgent)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	integration, _ := h.intRepo.GetByID(r.Context(), id)
	if platform == "tg" {
		go h.tgManager.StartIntegration(h.rootCtx, *integration)
	} else if platform == "hh" {
		logger.Debug(logger.HH, "[Web] Starting HH worker for new integration %s", identifier)
		go h.hhManager.StartIntegration(h.rootCtx, *integration)
	}

	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	integration, err := h.intRepo.GetByID(r.Context(), id)
	if err != nil {
		http.Error(w, "Integration not found", 404)
		return
	}

	newStatus := "active"
	if integration.Status == "active" {
		newStatus = "inactive"
		if integration.Platform == "tg" {
			h.tgManager.StopIntegration(integration.ID)
		} else if integration.Platform == "hh" {
			h.hhManager.StopIntegration(integration.ID)
		}
	} else {
		if integration.Platform == "tg" {
			go h.tgManager.StartIntegration(h.rootCtx, *integration)
		} else if integration.Platform == "hh" {
			go h.hhManager.StartIntegration(h.rootCtx, *integration)
		}
	}

	h.intRepo.UpdateStatus(r.Context(), id, newStatus)
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	integration, err := h.intRepo.GetByID(r.Context(), idStr)
	if err != nil {
		http.Redirect(w, r, "/accounts", 303)
		return
	}

	if integration.Platform == "tg" {
		h.tgManager.StopIntegration(integration.ID)
	} else if integration.Platform == "hh" {
		h.hhManager.StopIntegration(integration.ID)
	}

	if integration.Platform == "tg" && integration.SessionPath != "" {
		// Caution: os.Remove can fail if file doesn't exist, we ignore error
	}

	h.intRepo.Delete(r.Context(), idStr)
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

	integration, err := h.intRepo.GetByID(r.Context(), id)
	if err != nil {
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "Integration not found: %v"}`, err)))
		return
	}

	ua := ""
	if integration.UserAgent != nil {
		ua = *integration.UserAgent
	}

	token, err := hhclient.ExchangeToken(code, ua)
	if err != nil {
		w.Write([]byte(fmt.Sprintf(`{"ok": false, "error": "%v"}`, err)))
		return
	}

	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	h.intRepo.UpdateTokens(r.Context(), id, token.AccessToken, token.RefreshToken, expiresAt)

	integration, _ = h.intRepo.GetByID(r.Context(), id)
	go h.hhManager.StartIntegration(h.rootCtx, *integration)

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
