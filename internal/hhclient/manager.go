package hhclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
)

type Manager struct {
	cancels map[int64]context.CancelFunc

	db      *sqlx.DB
	conRepo *repository.ContactRepository
	msgRepo *repository.MessageRepository
	intRepo *repository.IntegrationRepository
}

func NewManager(db *sqlx.DB, conRepo *repository.ContactRepository, msgRepo *repository.MessageRepository, intRepo *repository.IntegrationRepository) *Manager {
	return &Manager{
		cancels: make(map[int64]context.CancelFunc),
		db:      db,
		conRepo: conRepo,
		msgRepo: msgRepo,
		intRepo: intRepo,
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
	logger.Debug(logger.Messaging, "[HH] [Int ID %d] Attempting to send message to neg %s", integrationID, negID)

	integration, err := m.intRepo.GetByID(ctx, integrationID)
	if err != nil {
		return err
	}

	urlVal := fmt.Sprintf("%snegotiations/%s/messages", HHApiURL, negID)
	logger.Debug(logger.Messaging, "[HH] [Int ID %d] URL: %s", integrationID, urlVal)

	data := url.Values{}
	data.Set("message", text)

	req, _ := http.NewRequestWithContext(ctx, "POST", urlVal, strings.NewReader(data.Encode()))
	req.Header.Set("Authorization", "Bearer "+*integration.AccessToken)
	req.Header.Set("User-Agent", *integration.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-HH-App-Active", "true")
	req.Header.Set("X-Idempotency-Key", uuid.New().String())

	logger.Debug(logger.Messaging, "[HH] [Int ID %d] Request Body: %s", integrationID, data.Encode())

	client := GetHHHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug(logger.Messaging, "[HH] [Int ID %d] Send error: %v", integrationID, err)
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	logger.Debug(logger.Messaging, "[HH] [Int ID %d] Response Status: %d, Body: %s", integrationID, resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hh api error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (m *Manager) Sync(ctx context.Context, integrationID int64) error {
	integration, err := m.intRepo.GetByID(ctx, integrationID)
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
		m.intRepo.UpdateTokens(ctx, integrationID, tr.AccessToken, tr.RefreshToken, expiresAt)
		integration.AccessToken = &tr.AccessToken
	}

	return m.syncNegotiations(ctx, *integration)
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

		// Save total applications count (from the first page response)
		if page == 0 && data.Found > 0 {
			stats := &models.NegotiationStats{
				IntegrationID:     integration.ID,
				ApplicationsCount: data.Found,
				UpdatedAt:         time.Now(),
			}
			// Use internal repository if available or we can use the db directly for this simple update
			_, _ = m.db.NamedExecContext(ctx, `INSERT INTO negotiations_stats (integration_id, applications_count, updated_at)
				VALUES (:integration_id, :applications_count, :updated_at)
				ON CONFLICT(integration_id) DO UPDATE SET
				applications_count=excluded.applications_count, updated_at=excluded.updated_at`, stats)
		}

		for _, item := range data.Items {
			// 1. Ensure Contact (using Negotiation ID to keep separate)
			contactID, err := m.getOrCreateContact(integration.ID, item.ID, item.Vacancy.Employer.Name, item.Vacancy.Name, item.State.Name)
			if err != nil {
				continue
			}

			// 2. Delta Sync check: compare UpdatedAt from HH with last message in our DB
			lastMsgTime, _ := m.msgRepo.GetLastMessageTimeByContactID(ctx, contactID)
			hhUpdateTime, _ := time.Parse(time.RFC3339, item.UpdatedAt)

			if hhUpdateTime.UTC().Format("2006-01-02 15:04:05") <= lastMsgTime {
				logger.Debug(logger.HH, "[HH] Skipping sync for negotiation %s (no changes since %s)", item.ID, lastMsgTime)
				continue
			}

			// 3. Sync Messages for this negotiation
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

	if err := m.conRepo.UpsertHHContact(context.Background(), integrationID, externalID, employerName, vacancyName, stateName); err != nil {
		return 0, err
	}

	return m.conRepo.GetIDByExternalID(context.Background(), "hh", externalID)
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
	// Start a transaction for batch insertion
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, msg := range data.Items {
		ts, err := time.Parse("2006-01-02T15:04:05-0700", msg.CreatedAt)
		if err != nil {
			ts, _ = time.Parse(time.RFC3339, msg.CreatedAt)
		}
		isIncoming := msg.Author.ParticipantType == "employer"

		if err := m.msgRepo.CreateExt(ctx, tx, integration.ID, contactID, fmt.Sprintf("hh_msg_%s", msg.ID), msg.Text, isIncoming, ts.UTC().Format("2006-01-02 15:04:05")); err != nil {
			logger.Debug(logger.HH, "[HH] DB error saving message: %v", err)
			continue
		}
		newMsgs++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	if newMsgs > 0 {
		logger.Debug(logger.HH, "[HH] Saved %d new messages from contact %d", newMsgs, contactID)
	}

	return nil
}
