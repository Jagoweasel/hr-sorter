package hh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hr-sorter/internal/logger"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HHClientWrapper implements the HHClient interface.
// It manages the HTTP client and token refresh logic.
type HHClientWrapper struct {
	httpClient *http.Client
	storage    SessionStorage
	config     ClientConfig
	apiBaseURL string
	authURL    string
	refreshMu  sync.Map // Map of accountID -> *sync.Mutex
}

// NewHHClientWrapper creates a new instance of the HHClientWrapper.
func NewHHClientWrapper(storage SessionStorage, config ClientConfig) HHClient {
	return &HHClientWrapper{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		storage:    storage,
		config:     config,
		apiBaseURL: "https://api.hh.ru",
		authURL:    "https://hh.ru/oauth/token",
	}
}

func (c *HHClientWrapper) getRefreshMu(accountID int64) *sync.Mutex {
	mu, _ := c.refreshMu.LoadOrStore(accountID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (c *HHClientWrapper) ExecuteRequest(ctx context.Context, session *Session, method string, path string, body interface{}) ([]byte, error) {
	access, _, expires := session.GetTokens()
	userAgent := session.GetUserAgent()

	logger.Trace(logger.HH, "[HHAPI] Request: %s %s (Account: %d)", method, path, session.AccountID)

	// If token is expired, refresh first
	if !expires.IsZero() && time.Now().After(expires) {
		logger.Debug(logger.HH, "[HHAPI] Token expired for account %d, refreshing before request...", session.AccountID)
		if err := c.RefreshToken(ctx, session); err != nil {
			return nil, fmt.Errorf("pre-request refresh failed: %w", err)
		}
		access, _, _ = session.GetTokens()
	}

	for retry := 0; retry < 2; retry++ {
		fullURL := c.apiBaseURL + path
		var bodyReader io.Reader
		if body != nil {
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal request body: %w", err)
			}
			bodyReader = bytes.NewReader(jsonData)
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set("User-Agent", userAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			logger.Error(logger.HH, "[HHAPI] Request failed: %v", err)
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		logger.Trace(logger.HH, "[HHAPI] Response: %d (Account: %d)", resp.StatusCode, session.AccountID)

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			if retry == 0 {
				logger.Debug(logger.HH, "[HHAPI] Auth error (%d), attempting token refresh for account %d...", resp.StatusCode, session.AccountID)
				if err := c.RefreshToken(ctx, session); err != nil {
					return nil, fmt.Errorf("refresh failed after 403/401: %w", err)
				}
				access, _, _ = session.GetTokens()
				continue // retry
			}
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("api error (%d): %s", resp.StatusCode, string(data))
		}

		return data, nil
	}

	return nil, fmt.Errorf("maximum retries exceeded")
}

func (c *HHClientWrapper) RefreshToken(ctx context.Context, session *Session) error {
	mu := c.getRefreshMu(session.AccountID)
	mu.Lock()
	defer mu.Unlock()

	// Re-check expiry after acquiring lock
	_, refresh, expires := session.GetTokens()
	if !expires.IsZero() && time.Now().Before(expires.Add(-time.Minute)) {
		logger.Debug(logger.HH, "[HHAPI] Token already refreshed by another goroutine for account %d", session.AccountID)
		return nil // Already refreshed by another goroutine
	}

	logger.Info(logger.HH, "[HHAPI] Refreshing token for account %d...", session.AccountID)

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refresh)
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", c.authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", session.GetUserAgent())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read refresh response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tr TokenResponse
	if err := json.Unmarshal(bodyBytes, &tr); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	session.SetTokens(tr.AccessToken, tr.RefreshToken, time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second))

	if err := c.storage.Save(ctx, session); err != nil {
		return fmt.Errorf("failed to save refreshed session: %w", err)
	}

	return nil
}
