package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/tgclient"
	"strings"
	"time"
	"unicode"
)

type ContactService struct {
	conRepo   *repository.ContactRepository
	fltRepo   *repository.FilterRepository
	msgRepo   *repository.MessageRepository
	tgManager *tgclient.Manager
	hhManager *hhclient.Manager
}

func NewContactService(conRepo *repository.ContactRepository, fltRepo *repository.FilterRepository, msgRepo *repository.MessageRepository, tgManager *tgclient.Manager, hhManager *hhclient.Manager) *ContactService {
	return &ContactService{
		conRepo:   conRepo,
		fltRepo:   fltRepo,
		msgRepo:   msgRepo,
		tgManager: tgManager,
		hhManager: hhManager,
	}
}

func (s *ContactService) SendChatMessage(ctx context.Context, contactID string, text string) error {
	// 1. Get contact details
	contact, err := s.conRepo.GetByID(ctx, contactID)
	if err != nil {
		return fmt.Errorf("failed to get contact: %w", err)
	}

	var externalID string
	if contact.Platform == "tg" {
		// For TG, externalID is the user ID
		username := ""
		if contact.Username != nil {
			username = *contact.Username
		}
		msgID, err := s.tgManager.SendMessage(ctx, contact.IntegrationID, contact.ExternalID, contact.AccessHash, username, text)
		if err != nil {
			return fmt.Errorf("tg send failed: %w", err)
		}
		externalID = fmt.Sprintf("%d", msgID)
	} else if contact.Platform == "hh" {
		// For HH, externalID is hh_neg_{negID}
		negID := strings.TrimPrefix(contact.ExternalID, "hh_neg_")
		err := s.hhManager.SendMessage(ctx, contact.IntegrationID, negID, text)
		if err != nil {
			return fmt.Errorf("hh send failed: %w", err)
		}
		// HH message IDs are handled during sync, so we use a temporary placeholder or wait for sync
		externalID = fmt.Sprintf("sent_%d", time.Now().Unix())
	} else {
		return fmt.Errorf("unsupported platform: %s", contact.Platform)
	}

	// 2. Save the outgoing message to the database
	timestamp := time.Now().UTC().Format("2006-01-02 15:04:05")
	err = s.msgRepo.Create(ctx, contact.IntegrationID, contact.ID, externalID, text, false, timestamp)
	if err != nil {
		// Don't fail the whole operation if DB save fails, but log it
		logger.Error(logger.HH, "DB error saving sent message: %v", err)
	}

	return nil
}

func (s *ContactService) GetFilteredContacts(ctx context.Context, accountID, platform string, showDeclines, hideScreened, hideUnanswered, showIgnored bool, sequenceFilter string) ([]repository.ContactWithLastMsg, error) {
	allContacts, err := s.conRepo.GetAll(ctx, accountID, platform, showDeclines)
	if err != nil {
		return nil, err
	}

	var activePatterns []string
	patterns, _ := s.fltRepo.GetActivePatterns(ctx)
	for _, p := range patterns {
		activePatterns = append(activePatterns, normalize(p))
	}

	var filtered []repository.ContactWithLastMsg
	for _, c := range allContacts {
		if c.IsIgnored != showIgnored {
			continue
		}

		if sequenceFilter == "with" && !c.InSequence {
			continue
		}
		if sequenceFilter == "without" && c.InSequence {
			continue
		}

		if c.Platform == "hh" {
			// Calculate IsFiltered
			if len(activePatterns) > 0 {
				normMsg := normalize(c.LastMessage)
				for _, p := range activePatterns {
					if strings.Contains(normMsg, p) {
						c.IsFiltered = true
						break
					}
				}
			}

			if hideUnanswered {
				if c.MsgCount == 0 || !c.LastIsIncoming {
					continue
				}
			}

			if hideScreened && c.IsFiltered {
				continue
			}
		}
		filtered = append(filtered, c)
	}

	return filtered, nil
}

func normalize(s string) string {
	f := func(r rune) bool { return unicode.IsSpace(r) }
	words := strings.FieldsFunc(s, f)
	return strings.ToLower(strings.Join(words, " "))
}

func (s *ContactService) UpdateIgnored(ctx context.Context, id interface{}, ignored bool) error {
	return s.conRepo.UpdateIgnored(ctx, id, ignored)
}
