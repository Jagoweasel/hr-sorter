package service

import (
	"context"
	"hr-sorter/internal/repository"
	"strings"
	"unicode"
)

type ContactService struct {
	conRepo *repository.ContactRepository
	fltRepo *repository.FilterRepository
}

func NewContactService(conRepo *repository.ContactRepository, fltRepo *repository.FilterRepository) *ContactService {
	return &ContactService{
		conRepo: conRepo,
		fltRepo: fltRepo,
	}
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
