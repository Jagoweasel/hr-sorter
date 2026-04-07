package web

import (
	"encoding/json"
	"fmt"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"net/http"
	"strconv"
	"time"
)

func (h *Handler) handleIntegrationStats(w http.ResponseWriter, r *http.Request) {
	tgCount := h.tgManager.GetActiveCount()
	hhCount := h.hhManager.GetActiveCount()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"active_count": tgCount + hhCount,
		"tg_active":    tgCount > 0,
		"hh_active":    hhCount > 0,
	})
}

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
		h.setHXLocation(w, "/accounts")
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
		h.setHXLocation(w, "/accounts")
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
		h.setHXLocation(w, "/accounts")
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

	logger.Debug(logger.HH, "[Web] Integration details: ID=%s, Platform=%s, Identifier=%s, HHAuthServiceAvailable=%v",
		idStr, integration.Platform, integration.Identifier, h.hhAuthService != nil)

	if integration.Platform == "hh" && h.hhAuthService != nil {
		logger.Info(logger.HH, "[Web] Launching HH Auth Service for %s (Integration ID: %s, Account ID: %d)...",
			integration.Identifier, idStr, integration.AccountID)
		accID := integration.AccountID
		go func() {
			_, err := h.hhAuthService.StartFlow(h.rootCtx, accID, integration.Identifier)
			if err != nil {
				logger.Error(logger.HH, "[Web] Manual start HH auth failed for %s (ID: %s): %v",
					integration.Identifier, idStr, err)
			}
		}()
	} else {

		if integration.Platform != "hh" {
			logger.Error(logger.HH, "[Web] Cannot start auth: Platform is %s, integration ID %s (expected hh)",
				integration.Platform, idStr)
		}
		if h.hhAuthService == nil {
			logger.Error(logger.HH, "[Web] Cannot start HH auth: HHAuthService is nil. Check startup logs for Playwright/Initialization errors.")
		}
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
	if integration.Platform == "hh" {
		authURL = hhclient.GetAuthorizeURL()
	}

	// If it's HH and pending_auth, check the live auth service state
	if integration.Platform == "hh" && h.hhAuthService != nil && (status == "pending_auth" || status == "awaiting_code") {
		if flow, ok := h.hhAuthService.GetFlow(integration.AccountID); ok {
			liveStatus := flow.GetStatus()
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

func (h *Handler) handleResetHHAuth(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	integration, err := h.intRepo.GetByID(r.Context(), idStr)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}

	if integration.Platform == "hh" && h.hhAuthService != nil {
		logger.Info(logger.HH, "[Web] Resetting HH Auth for %s...", integration.Identifier)
		if flow, ok := h.hhAuthService.GetFlow(integration.AccountID); ok {
			_ = flow.Cancel()
		}
		// Small delay to allow cleanup
		time.Sleep(500 * time.Millisecond)

		go func() {
			_, err := h.hhAuthService.StartFlow(h.rootCtx, integration.AccountID, integration.Identifier)
			if err != nil {
				logger.Error(logger.HH, "[Web] Manual restart HH auth failed for %s: %v", integration.Identifier, err)
			}
		}()
	}

	w.WriteHeader(http.StatusOK)
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
