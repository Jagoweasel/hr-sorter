package hhclient

import (
	"context"
	"encoding/json"
	"fmt"
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
	logger.Debug(logger.HH, "[HH] Fetching negotiations for %s", integration.Identifier)
	req, _ := http.NewRequestWithContext(ctx, "GET", HHApiURL+"negotiations", nil)
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
			ID        string `json:"id"`
			UpdatedAt string `json:"updated_at"`
			Employer  struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"employer"`
			Vacancy struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"vacancy"`
			MessagesURL string `json:"messages_url"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	logger.Debug(logger.HH, "[HH] Found %d negotiations for %s", len(data.Items), integration.Identifier)

	for _, item := range data.Items {
		// 1. Ensure Contact
		contactID, err := m.getOrCreateContact(integration.ID, item.Employer.ID, item.Employer.Name, item.Vacancy.Name)
		if err != nil {
			continue
		}

		// 2. Sync Messages for this negotiation
		if err := m.syncMessages(ctx, integration, contactID, item.MessagesURL); err != nil {
			logger.Debug(logger.HH, "[HH] Failed to sync messages for negotiation %s: %v", item.ID, err)
		}
	}

	return nil
}

func (m *Manager) getOrCreateContact(integrationID int64, employerID, employerName, vacancyName string) (int64, error) {
	externalID := fmt.Sprintf("hh_%s", employerID)
	_, err := database.DB.Exec(`
		INSERT INTO contacts (integration_id, platform, external_id, first_name, username) 
		VALUES (?, 'hh', ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET first_name = excluded.first_name`,
		integrationID, externalID, employerName, vacancyName)
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
				ParticipantGroup string `json:"participant_group"` // "employer" or "applicant"
			} `json:"author"`
		} `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	newMsgs := 0
	for _, msg := range data.Items {
		ts, _ := time.Parse(time.RFC3339, msg.CreatedAt)
		isIncoming := msg.Author.ParticipantGroup == "employer"

		res, err := database.DB.Exec(`
			INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			integration.ID, contactID, fmt.Sprintf("hh_%s", msg.ID), msg.Text, isIncoming, ts)
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
