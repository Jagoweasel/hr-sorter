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

	logger.Debug(logger.Sync, "[HH] Starting sync for integration %s", integration.Identifier)

	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// Initial sync
		if err := m.Sync(intCtx, integration.ID); err != nil {
			log.Printf("[HH] Initial sync failed for %s: %v", integration.Identifier, err)
		}

		for {
			select {
			case <-intCtx.Done():
				return
			case <-ticker.C:
				if err := m.Sync(intCtx, integration.ID); err != nil {
					log.Printf("[HH] Sync failed for %s: %v", integration.Identifier, err)
				}
			}
		}
	}()

	return nil
}

func (m *Manager) Sync(ctx context.Context, integrationID int64) error {
	var integration models.Integration
	err := database.DB.Get(&integration, "SELECT * FROM integrations WHERE id = ?", integrationID)
	if err != nil {
		return err
	}

	if integration.Status != "active" || integration.AccessToken == nil {
		return nil
	}

	// Check if token needs refresh (5 min buffer)
	if integration.ExpiresAt != nil && time.Now().Add(5*time.Minute).After(*integration.ExpiresAt) {
		logger.Debug(logger.Sync, "[HH] Refreshing token for %s", integration.Identifier)
		tr, err := RefreshToken(*integration.RefreshToken)
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
	req, _ := http.NewRequestWithContext(ctx, "GET", HHApiURL+"negotiations", nil)
	req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
	req.Header.Set("User-Agent", *integration.UserAgent)
	req.Header.Set("X-HH-App-Active", "true")

	resp, err := http.DefaultClient.Do(req)
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

	for _, item := range data.Items {
		// 1. Ensure Contact
		contactID, err := m.getOrCreateContact(integration.ID, item.Employer.ID, item.Employer.Name, item.Vacancy.Name)
		if err != nil {
			continue
		}

		// 2. Sync Messages for this negotiation
		if err := m.syncMessages(ctx, integration, contactID, item.MessagesURL); err != nil {
			log.Printf("[HH] Failed to sync messages for negotiation %s: %v", item.ID, err)
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
	req, _ := http.NewRequestWithContext(ctx, "GET", messagesURL, nil)
	req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
	req.Header.Set("User-Agent", *integration.UserAgent)
	req.Header.Set("X-HH-App-Active", "true")

	resp, err := http.DefaultClient.Do(req)
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

	for _, msg := range data.Items {
		ts, _ := time.Parse(time.RFC3339, msg.CreatedAt)
		isIncoming := msg.Author.ParticipantGroup == "employer"

		_, err := database.DB.Exec(`
			INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?)`,
			integration.ID, contactID, fmt.Sprintf("hh_%s", msg.ID), msg.Text, isIncoming, ts)
		if err != nil {
			log.Printf("[HH] DB error saving message: %v", err)
		}
	}

	return nil
}
