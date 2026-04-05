package web

import (
	"fmt"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/logger"
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

	if r.Header.Get("HX-Request") != "" {
		w.Header().Set("HX-Location", "/accounts")
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, "/accounts", 303)
}

func (h *Handler) handleToggleIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	if err := h.intService.ToggleIntegration(r.Context(), id, h.rootCtx); err != nil {
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

func (h *Handler) handleDeleteIntegration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := r.URL.Query().Get("id")
	if err := h.intService.DeleteIntegration(r.Context(), idStr); err != nil {
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

func (h *Handler) handleStartAuth(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	logger.Info(logger.HH, "[Web] StartAuth requested for integration ID: %s", idStr)
	integration, err := h.intRepo.GetByID(r.Context(), idStr)
	if err != nil {
		logger.Error(logger.HH, "[Web] Failed to fetch integration %s: %v", idStr, err)
		http.Error(w, err.Error(), 500)
		return
	}

	if integration.Platform == "hh" && h.hhAuthService != nil {
		logger.Info(logger.HH, "[Web] Launching HH Auth Service for %s...", integration.Identifier)
		accID := integration.AccountID
		go func() {
			_, err := h.hhAuthService.StartAuth(h.rootCtx, integration.Identifier, accID)
			if err != nil {
				logger.Error(logger.HH, "[Web] Manual start HH auth failed for %s: %v", integration.Identifier, err)
			}
		}()
	} else {
		logger.Info(logger.HH, "[Web] Cannot start HH auth: Service is nil or platform is not HH")
	}

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleIntegrationStatus(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	integration, err := h.intRepo.GetByID(r.Context(), idStr)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status": "error"}`))
		return
	}

	status := integration.Status
	authURL := ""

	// If it's HH and pending_auth, check the live auth service state
	if integration.Platform == "hh" && h.hhAuthService != nil && (status == "pending_auth" || status == "awaiting_code") {
		liveStatus, _ := h.hhAuthService.GetStatus(r.Context())
		if liveStatus != nil {
			switch liveStatus.State {
			case dto.AuthStateWaitOTP:
				status = "awaiting_code"
			case dto.AuthStateWaitCaptcha:
				status = "awaiting_captcha"
			case dto.AuthStateCompleted:
				status = "active"
			case dto.AuthStateFailed:
				status = "failed"
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	h.templates.Render(w, r, "fragments/status.json", struct {
		Status     string `json:"status"`
		Identifier string `json:"identifier"`
		Platform   string `json:"platform"`
		AuthURL    string `json:"auth_url"`
	}{
		Status:     status,
		Identifier: integration.Identifier,
		Platform:   integration.Platform,
		AuthURL:    authURL,
	}, h.getLocale(r))
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
