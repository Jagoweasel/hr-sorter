package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/tgclient"
	"time"
)

type IntegrationService struct {
	intRepo   *repository.IntegrationRepository
	tgManager *tgclient.Manager
	hhManager *hhclient.Manager
}

func NewIntegrationService(intRepo *repository.IntegrationRepository, tgManager *tgclient.Manager, hhManager *hhclient.Manager) *IntegrationService {
	return &IntegrationService{
		intRepo:   intRepo,
		tgManager: tgManager,
		hhManager: hhManager,
	}
}

func (s *IntegrationService) CreateIntegration(ctx context.Context, accID, platform, identifier string, apiID int, apiHash string, rootCtx context.Context) error {
	status := "pending_auth"
	sessionPath := ""
	var userAgent *string
	if platform == "tg" {
		sessionPath = fmt.Sprintf("sessions/%s.json", identifier)
	} else if platform == "hh" {
		ua := hhclient.GenerateAndroidUserAgent()
		userAgent = &ua
	}

	id, err := s.intRepo.Create(ctx, accID, platform, identifier, apiID, apiHash, status, sessionPath, userAgent)
	if err != nil {
		return err
	}

	integration, err := s.intRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if platform == "tg" {
		go s.tgManager.StartIntegration(rootCtx, *integration)
	} else if platform == "hh" {
		logger.Debug(logger.HH, "[Service] Starting HH worker for new integration %s", identifier)
		go s.hhManager.StartIntegration(rootCtx, *integration)
	}

	return nil
}

func (s *IntegrationService) ToggleIntegration(ctx context.Context, id string, rootCtx context.Context) error {
	integration, err := s.intRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	newStatus := "active"
	if integration.Status == "active" {
		newStatus = "inactive"
		if integration.Platform == "tg" {
			s.tgManager.StopIntegration(integration.ID)
		} else if integration.Platform == "hh" {
			s.hhManager.StopIntegration(integration.ID)
		}
	} else {
		if integration.Platform == "tg" {
			go s.tgManager.StartIntegration(rootCtx, *integration)
		} else if integration.Platform == "hh" {
			go s.hhManager.StartIntegration(rootCtx, *integration)
		}
	}

	return s.intRepo.UpdateStatus(ctx, id, newStatus)
}

func (s *IntegrationService) DeleteIntegration(ctx context.Context, id string) error {
	integration, err := s.intRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if integration.Platform == "tg" {
		s.tgManager.StopIntegration(integration.ID)
	} else if integration.Platform == "hh" {
		s.hhManager.StopIntegration(integration.ID)
	}

	return s.intRepo.Delete(ctx, id)
}

func (s *IntegrationService) HandleHHAuth(ctx context.Context, id int64, code string, rootCtx context.Context) error {
	integration, err := s.intRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	ua := ""
	if integration.UserAgent != nil {
		ua = *integration.UserAgent
	}

	token, err := hhclient.ExchangeToken(code, ua)
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)
	if err := s.intRepo.UpdateTokens(ctx, id, token.AccessToken, token.RefreshToken, expiresAt); err != nil {
		return err
	}

	integration, err = s.intRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	go s.hhManager.StartIntegration(rootCtx, *integration)

	return nil
}
