package hhclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"hr-sorter/internal/database"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
)

type Manager struct {
	cancels map[int64]context.CancelFunc
}

func NewManager() *Manager {
	return &Manager{
		cancels: make(map[int64]context.CancelFunc),
	}
}

func (m *Manager) StartIntegration(ctx context.Context, integration models.Integration) error {
	intCtx, cancel := context.WithCancel(ctx)
	m.cancels[integration.ID] = cancel

	log.Printf("[HH] Initializing background sync for %s (ID: %d)", integration.Identifier, integration.ID)
	logger.Debug(logger.HH, "[HH] Starting sync loop for integration %s", integration.Identifier)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// Initial sync
		logger.Debug(logger.HH, "[HH] Triggering initial sync for %s", integration.Identifier)
		if err := m.Sync(intCtx, integration.ID); err != nil {
			logger.Debug(logger.HH, "[HH] Initial sync failed for %s: %v", integration.Identifier, err)
		}

		for {
			select {
			case <-intCtx.Done():
				logger.Debug(logger.HH, "[HH] Stopping sync for %s (context cancelled)", integration.Identifier)
				return
			case <-ticker.C:
				logger.Debug(logger.HH, "[HH] Ticker triggered sync for %s", integration.Identifier)
				if err := m.Sync(intCtx, integration.ID); err != nil {
					logger.Debug(logger.HH, "[HH] Sync failed for %s: %v", integration.Identifier, err)
				}
			}
		}
	}()

	return nil
}

func (m *Manager) StopIntegration(id int64) {
	if cancel, ok := m.cancels[id]; ok {
		cancel()
		delete(m.cancels, id)
		logger.Debug(logger.HH, "[HH] Stopped background manager for integration %d", id)
	}
}

func (m *Manager) SendMessage(ctx context.Context, integrationID int64, negID string, text string) error {
	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", integrationID)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%snegotiations/%s/messages", HHApiURL, negID)
	// Body for HH API is usually text=... or JSON.
	// The User's log showed JSON, but it was for chatik.
	// Official API: POST /negotiations/{negotiation_id}/messages

	body := map[string]string{
		"text": text,
	}
	jsonBody, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBody))
	req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
	req.Header.Set("User-Agent", *integration.UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-HH-App-Active", "true")

	client := GetHHHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		// Log error body
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("hh api error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (m *Manager) Sync(ctx context.Context, integrationID int64) error {
	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", integrationID)
	if err != nil {
		return err
	}

	if integration.Status != "active" {
		logger.Debug(logger.HH, "[HH] Skipping sync for %s: status is %s (need 'active')", integration.Identifier, integration.Status)
		return nil
	}
	if integration.AccessToken == nil || *integration.AccessToken == "" {
		logger.Debug(logger.HH, "[HH] Skipping sync for %s: access token is missing or empty", integration.Identifier)
		return nil
	}

	// Check if token needs refresh (5 min buffer)
	if integration.ExpiresAt != nil && time.Now().Add(5*time.Minute).After(*integration.ExpiresAt) {
		logger.Debug(logger.HH, "[HH] Refreshing token for %s", integration.Identifier)
		ua := ""
		if integration.UserAgent != nil {
			ua = *integration.UserAgent
		}
		tr, err := RefreshToken(*integration.RefreshToken, ua)
		if err != nil {
			return fmt.Errorf("refresh token: %w", err)
		}
		expiresAt := time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
		database.DB.Exec("UPDATE integrations SET access_token = ?, refresh_token = ?, expires_at = ? WHERE id = ?",
			tr.AccessToken, tr.RefreshToken, expiresAt, integrationID)
		integration.AccessToken = &tr.AccessToken
	}

	return m.syncNegotiations(ctx, integration)
}

func (m *Manager) syncNegotiations(ctx context.Context, integration models.Integration) error {
	page := 0
	for {
		logger.Debug(logger.HH, "[HH] Fetching negotiations page %d for %s", page, integration.Identifier)
		url := fmt.Sprintf("%snegotiations?page=%d&per_page=20", HHApiURL, page)
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
		req.Header.Set("User-Agent", *integration.UserAgent)
		req.Header.Set("X-HH-App-Active", "true")

		client := GetHHHttpClient()
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("api error: %d", resp.StatusCode)
		}

		var data struct {
			Items []struct {
				ID    string `json:"id"`
				State struct {
					Name string `json:"name"`
				} `json:"state"`
				UpdatedAt string `json:"updated_at"`
				Vacancy   struct {
					Name     string `json:"name"`
					Employer struct {
						Name string `json:"name"`
					} `json:"employer"`
				} `json:"vacancy"`
				MessagesURL string `json:"messages_url"`
			} `json:"items"`
			Found int `json:"found"`
			Pages int `json:"pages"`
			Page  int `json:"page"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return err
		}

		logger.Debug(logger.HH, "[HH] Found %d negotiations on page %d for %s", len(data.Items), data.Page, integration.Identifier)

		for _, item := range data.Items {
			// 1. Ensure Contact (using Negotiation ID to keep separate)
			contactID, err := m.getOrCreateContact(integration.ID, item.ID, item.Vacancy.Employer.Name, item.Vacancy.Name, item.State.Name)
			if err != nil {
				continue
			}

			// 2. Sync Messages for this negotiation
			if err := m.syncMessages(ctx, integration, contactID, item.MessagesURL); err != nil {
				logger.Debug(logger.HH, "[HH] Failed to sync messages for negotiation %s: %v", item.ID, err)
			}
		}

		if data.Page >= data.Pages-1 || len(data.Items) == 0 {
			break
		}
		page++
	}

	return nil
}

func (m *Manager) getOrCreateContact(integrationID int64, negID, employerName, vacancyName, stateName string) (int64, error) {
	externalID := fmt.Sprintf("hh_neg_%s", negID)
	logger.Debug(logger.HH, "[HH] Ensuring contact for negotiation %s (%s - %s)", negID, employerName, vacancyName)
	// We map:
	// first_name -> Employer Name
	// last_name -> Vacancy Name
	// username -> Current Status (State)
	// access_hash -> Always 0 for HH
	_, err := database.DB.Exec(`
		INSERT INTO contacts (integration_id, platform, external_id, first_name, last_name, username, access_hash) 
		VALUES (?, 'hh', ?, ?, ?, ?, 0)
		ON CONFLICT(external_id) DO UPDATE SET 
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			username = excluded.username,
			access_hash = 0`,
		integrationID, externalID, employerName, vacancyName, stateName)
	if err != nil {
		return 0, err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE platform = 'hh' AND external_id = ?", externalID)
	return contactID, err
}

func (m *Manager) syncMessages(ctx context.Context, integration models.Integration, contactID int64, messagesURL string) error {
	logger.Debug(logger.HH, "[HH] Syncing messages from %s", messagesURL)
	req, _ := http.NewRequestWithContext(ctx, "GET", messagesURL, nil)
	req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
	req.Header.Set("User-Agent", *integration.UserAgent)
	req.Header.Set("X-HH-App-Active", "true")

	client := GetHHHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data struct {
		Items []struct {
			ID        string `json:"id"`
			CreatedAt string `json:"created_at"`
			Text      string `json:"text"`
			Author    struct {
				ParticipantType string `json:"participant_type"` // "employer" or "applicant"
			} `json:"author"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	newMsgs := 0
	for _, msg := range data.Items {
		// HH uses +0300 format which time.RFC3339 might not like depending on Go version
		// but usually it works. Let's use a more flexible parser.
		ts, err := time.Parse("2006-01-02T15:04:05-0700", msg.CreatedAt)
		if err != nil {
			// Fallback to RFC3339
			ts, _ = time.Parse(time.RFC3339, msg.CreatedAt)
		}
		isIncoming := msg.Author.ParticipantType == "employer"

		res, err := database.DB.Exec(`
			INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			integration.ID, contactID, fmt.Sprintf("hh_msg_%s", msg.ID), msg.Text, isIncoming, ts.UTC().Format("2006-01-02 15:04:05"))
		if err != nil {
			logger.Debug(logger.HH, "[HH] DB error saving message: %v", err)
			continue
		}

		if rows, _ := res.RowsAffected(); rows > 0 {
			newMsgs++
		}
	}

	if newMsgs > 0 {
		logger.Debug(logger.HH, "[HH] Saved %d new messages from contact %d", newMsgs, contactID)
	}

	return nil
}
