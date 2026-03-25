package service

import (
	"context"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/tgclient"
	"os"
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
