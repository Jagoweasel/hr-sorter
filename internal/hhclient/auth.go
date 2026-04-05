package hhclient

import (
	"context"
	"hr-sorter/internal/domain/dto"
	"sync"
)

// HHClient implementation using Playwright-go
type HHAuthService struct {
	mu     sync.Mutex
	status *dto.HHAuthStatus
	// browser *playwright.Browser
	// page *playwright.Page
}

func NewHHAuthService() *HHAuthService {
	return &HHAuthService{
		status: &dto.HHAuthStatus{State: dto.AuthStateNone},
	}
}

func (s *HHAuthService) StartAuth(ctx context.Context, identify string) (*dto.HHAuthStatus, error) {
	panic("implement me with Playwright-go")
}

func (s *HHAuthService) SubmitOTP(ctx context.Context, code string) (*dto.HHAuthStatus, error) {
	panic("implement me with Playwright-go")
}

func (s *HHAuthService) SubmitCaptcha(ctx context.Context, resolution string) (*dto.HHAuthStatus, error) {
	panic("implement me with Playwright-go")
}

func (s *HHAuthService) GetStatus(ctx context.Context) (*dto.HHAuthStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}

func (s *HHAuthService) FetchNegotiations(ctx context.Context, accountID string) ([]dto.HHNegotiation, error) {
	panic("implement me with HH API")
}
