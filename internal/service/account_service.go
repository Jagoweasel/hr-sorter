package service

import (
	"context"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/tgclient"
	"os"
	"regexp"
	"strings"
)

type AccountService struct {
	accRepo   *repository.AccountRepository
	intRepo   *repository.IntegrationRepository
	tgManager *tgclient.Manager
	hhManager *hhclient.Manager
}

func NewAccountService(accRepo *repository.AccountRepository, intRepo *repository.IntegrationRepository, tgManager *tgclient.Manager, hhManager *hhclient.Manager) *AccountService {
	return &AccountService{
		accRepo:   accRepo,
		intRepo:   intRepo,
		tgManager: tgManager,
		hhManager: hhManager,
	}
}

func (s *AccountService) CreateAccount(ctx context.Context, name string) error {
	slug := s.GenerateSlug(name)
	return s.accRepo.Create(ctx, name, slug)
}

func (s *AccountService) UpdateAccount(ctx context.Context, id, name, slug string) error {
	if slug == "" {
		slug = s.GenerateSlug(name)
	}
	return s.accRepo.Update(ctx, id, name, slug)
}

func (s *AccountService) GenerateSlug(name string) string {
	// Simple slugifier: lowercase, replace spaces/non-latin with underscores
	// Note: For real Cyrillic transliteration a library would be better,
	// but let's do a basic one for now or just filter.
	res := strings.ToLower(name)

	// Basic transliteration for common Cyrillic
	replacer := strings.NewReplacer(
		"а", "a", "б", "b", "в", "v", "г", "g", "д", "d", "е", "e", "ё", "yo",
		"ж", "zh", "з", "z", "и", "i", "й", "j", "к", "k", "л", "l", "м", "m",
		"н", "n", "о", "o", "п", "p", "р", "r", "с", "s", "т", "t", "у", "u",
		"ф", "f", "х", "h", "ц", "ts", "ч", "ch", "ш", "sh", "щ", "sch", "ъ", "",
		"ы", "y", "ь", "", "э", "e", "ю", "yu", "я", "ya",
	)
	res = replacer.Replace(res)

	reg := regexp.MustCompile("[^a-z0-9]+")
	res = reg.ReplaceAllString(res, "_")
	return strings.Trim(res, "_")
}

func (s *AccountService) ToggleAccount(ctx context.Context, id string, rootCtx context.Context) error {
	status, err := s.accRepo.GetStatus(ctx, id)
	if err != nil {
		return err
	}

	newStatus := "active"
	if status == "active" {
		newStatus = "inactive"
		integrations, err := s.intRepo.GetByAccountID(ctx, id)
		if err == nil {
			for _, i := range integrations {
				if i.Platform == "tg" {
					s.tgManager.StopIntegration(i.ID)
				} else if i.Platform == "hh" {
					s.hhManager.StopIntegration(i.ID)
				}
			}
		}
	} else {
		integrations, err := s.intRepo.GetByAccountID(ctx, id)
		if err == nil {
			for _, i := range integrations {
				if i.Status == "active" || i.Status == "pending_auth" {
					if i.Platform == "tg" {
						go s.tgManager.StartIntegration(rootCtx, i)
					} else if i.Platform == "hh" {
						go s.hhManager.StartIntegration(rootCtx, i)
					}
				}
			}
		}
	}

	return s.accRepo.UpdateStatus(ctx, id, newStatus)
}

func (s *AccountService) DeleteAccount(ctx context.Context, id string) error {
	integrations, err := s.intRepo.GetByAccountID(ctx, id)
	if err == nil {
		for _, i := range integrations {
			if i.Platform == "tg" {
				s.tgManager.StopIntegration(i.ID)
				if i.SessionPath != "" {
					os.Remove(i.SessionPath)
				}
			} else if i.Platform == "hh" {
				s.hhManager.StopIntegration(i.ID)
			}
		}
	}

	return s.accRepo.Delete(ctx, id)
}
