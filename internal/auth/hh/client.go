package hh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

func (c *HHClientWrapper) ExecuteRequest(ctx context.Context, session *Session, method string, path string, body interface{}) ([]byte, error) {
	// If token is expired, refresh first
	if !session.ExpiresAt.IsZero() && time.Now().After(session.ExpiresAt) {
		if err := c.RefreshToken(ctx, session); err != nil {
			return nil, fmt.Errorf("pre-request refresh failed: %w", err)
		}
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

		req.Header.Set("Authorization", "Bearer "+session.AccessToken)
		req.Header.Set("User-Agent", session.UserAgent)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			if retry == 0 {
				if err := c.RefreshToken(ctx, session); err != nil {
					return nil, fmt.Errorf("refresh failed after 403/401: %w", err)
				}
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
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", session.RefreshToken)
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", c.authURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", session.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh failed (%d): %s", resp.StatusCode, string(body))
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	session.AccessToken = tr.AccessToken
	session.RefreshToken = tr.RefreshToken
	session.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)

	if err := c.storage.Save(ctx, session); err != nil {
		return fmt.Errorf("failed to save refreshed session: %w", err)
	}

	return nil
}
