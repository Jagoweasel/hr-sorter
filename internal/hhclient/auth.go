package hhclient

import (
	"context"
	"fmt"
	"hr-sorter/internal/domain"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// HHAuthService implementation using Playwright-go
type HHAuthService struct {
	mu     sync.Mutex
	status *dto.HHAuthStatus
	repo   domain.Repository

	pw      *playwright.Playwright
	browser playwright.Browser
	// Per-session page and context
	sessionContext playwright.BrowserContext
	sessionPage    playwright.Page

	codeChan    chan string
	otpChan     chan string
	captchaChan chan string
	errChan     chan error
	stopChan    chan struct{}
	closeOnce   sync.Once

	isRunning bool
	identify  string
	accountID int64
}

func NewHHAuthService(repo domain.Repository) (*HHAuthService, error) {
	pw, err := playwright.Run()
	if err != nil {
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(false),
	})
	if err != nil {
		_ = pw.Stop()
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	return &HHAuthService{
		status:      &dto.HHAuthStatus{State: dto.AuthStateNone},
		repo:        repo,
		codeChan:    make(chan string, 1),
		otpChan:     make(chan string),
		captchaChan: make(chan string),
		errChan:     make(chan error),
		stopChan:    make(chan struct{}),
		pw:          pw,
		browser:     browser,
	}, nil
}

func (s *HHAuthService) Close() error {
	var errs []string
	if s.browser != nil {
		if err := s.browser.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("browser close error: %v", err))
		}
	}
	if s.pw != nil {
		if err := s.pw.Stop(); err != nil {
			errs = append(errs, fmt.Sprintf("playwright stop error: %v", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *HHAuthService) stop() {
	s.closeOnce.Do(func() {
		close(s.stopChan)
	})
}

func (s *HHAuthService) StartAuth(ctx context.Context, identify string, accountID int64) (*dto.HHAuthStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		logger.Info(logger.HH, "[HHAuth] Auth already running for %s", s.identify)
		return s.status, nil
	}

	logger.Info(logger.HH, "[HHAuth] Starting auth flow for %s (Acc ID %d, Browser: headless=false)", identify, accountID)
	s.status = &dto.HHAuthStatus{State: dto.AuthStateWaitIdentify}
	s.isRunning = true
	s.identify = identify
	s.accountID = accountID
	s.stopChan = make(chan struct{})
	s.closeOnce = sync.Once{}

	go s.runFlow(identify)

	return s.status, nil
}

func (s *HHAuthService) runFlow(identify string) {
	logger.Debug(logger.HH, "[HHAuth] runFlow started for %s", identify)
	defer func() {
		s.mu.Lock()
		s.isRunning = false
		if s.sessionPage != nil {
			s.sessionPage.Close()
			s.sessionPage = nil
		}
		if s.sessionContext != nil {
			s.sessionContext.Close()
			s.sessionContext = nil
		}
		s.mu.Unlock()
		logger.Info(logger.HH, "[HHAuth] Flow finished for %s", identify)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	userAgent := GenerateAndroidUserAgent()
	var err error
	logger.Debug(logger.HH, "[HHAuth] Creating browser context...")

	// Use standard device for browser profile
	device := s.pw.Devices["Pixel 7"]
	s.sessionContext, err = s.browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String(device.UserAgent),
		Viewport:          device.Viewport,
		DeviceScaleFactor: playwright.Float(device.DeviceScaleFactor),
		IsMobile:          playwright.Bool(device.IsMobile),
		HasTouch:          playwright.Bool(device.HasTouch),
	})
	if err != nil {
		s.fail(fmt.Errorf("failed to create context: %w", err))
		return
	}

	s.sessionPage, err = s.sessionContext.NewPage()
	if err != nil {
		s.fail(fmt.Errorf("failed to create page: %w", err))
		return
	}

	// Capture redirect to hhandroid://
	s.sessionContext.OnRequest(func(request playwright.Request) {
		if strings.HasPrefix(request.URL(), "hhandroid://") {
			logger.Info(logger.HH, "[HHAuth] Detected redirect: %s", request.URL())
			u, _ := url.Parse(request.URL())
			code := u.Query().Get("code")
			if code != "" {
				select {
				case s.codeChan <- code:
				default:
				}
			}
		}
	})

	authURL := GetAuthorizeURL()
	logger.Info(logger.HH, "[HHAuth] Navigating to %s", authURL)

	// Race between Goto and Code interception
	go func() {
		// Just trigger navigation, don't block main loop if it redirects quickly
		_, _ = s.sessionPage.Goto(authURL, playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateLoad,
		})
	}()

	// Check if we already got the code (immediate redirect)
	select {
	case code := <-s.codeChan:
		logger.Info(logger.HH, "[HHAuth] Captured code immediately after Goto trigger")
		s.complete(code, userAgent)
		return
	case <-time.After(5 * time.Second):
		// No immediate redirect, proceed to identification
	case <-s.stopChan:
		return
	}

	// 1. Identify
	logger.Debug(logger.HH, "[HHAuth] Entering login: %s", identify)
	err = s.handleIdentify(identify)
	if err != nil {
		s.fail(err)
		return
	}

	// Main flow loop
	for {
		// Check for code constantly in the background
		select {
		case code := <-s.codeChan:
			logger.Info(logger.HH, "[HHAuth] Captured code during flow")
			s.complete(code, userAgent)
			return
		case <-s.stopChan:
			return
		default:
		}

		logger.Debug(logger.HH, "[HHAuth] Detecting page state...")
		state, err := s.detectState(ctx)
		if err != nil {
			s.fail(err)
			return
		}

		switch state {
		case dto.AuthStateWaitOTP:
			logger.Info(logger.HH, "[HHAuth] State: Waiting for OTP from user")
			s.mu.Lock()
			s.status.State = dto.AuthStateWaitOTP
			s.mu.Unlock()
			select {
			case code := <-s.otpChan:
				logger.Debug(logger.HH, "[HHAuth] Received OTP, filling form...")
				if err := s.sessionPage.Fill("input[data-qa='magritte-pincode-input-field']", code); err != nil {
					_ = s.sessionPage.Fill("input[name='code']", code)
				}
				if err := s.sessionPage.Keyboard().Press("Enter"); err != nil {
					s.fail(fmt.Errorf("failed to submit OTP: %w", err))
					return
				}
			case <-ctx.Done():
				s.fail(fmt.Errorf("auth timeout"))
				return
			case <-s.stopChan:
				return
			}
		case dto.AuthStateWaitCaptcha:
			logger.Info(logger.HH, "[HHAuth] State: Waiting for Captcha from user")
			s.mu.Lock()
			s.status.State = dto.AuthStateWaitCaptcha
			imgElement := s.sessionPage.Locator("img[data-qa='account-captcha-picture']")
			if visible, _ := imgElement.IsVisible(); !visible {
				imgElement = s.sessionPage.Locator("img[src*='captcha']")
			}
			screenshot, _ := imgElement.Screenshot()
			s.status.CaptchaImg = screenshot
			s.mu.Unlock()
			select {
			case resolution := <-s.captchaChan:
				logger.Debug(logger.HH, "[HHAuth] Received Captcha, filling form...")
				if err := s.sessionPage.Fill("input[data-qa='account-captcha-input']", resolution); err != nil {
					_ = s.sessionPage.Fill("input[name='captcha']", resolution)
				}
				if err := s.sessionPage.Keyboard().Press("Enter"); err != nil {
					s.fail(fmt.Errorf("failed to submit captcha: %w", err))
					return
				}
			case <-ctx.Done():
				s.fail(fmt.Errorf("auth timeout"))
				return
			case <-s.stopChan:
				return
			}
		default:
			// Check if we already succeeded or stopped
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				s.fail(fmt.Errorf("auth timeout"))
				return
			case <-time.After(2 * time.Second):
				continue
			}
		}
	}
}

func (s *HHAuthService) detectState(ctx context.Context) (dto.HHAuthState, error) {
	// Wait for any selector that indicates a state change
	_, err := s.sessionPage.WaitForSelector("input[data-qa='magritte-pincode-input-field'], input[name='code'], img[data-qa='account-captcha-picture'], img[src*='captcha'], div.bloko-notification--error, div[data-qa='login-error']", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(5000),
	})
	if err != nil {
		if strings.Contains(err.Error(), "timeout") {
			return dto.AuthStateNone, nil
		}
		return dto.AuthStateNone, fmt.Errorf("wait for selector failed: %w", err)
	}

	if visible, _ := s.sessionPage.Locator("div.bloko-notification--error").IsVisible(); visible {
		msg, _ := s.sessionPage.Locator("div.bloko-notification--error").InnerText()
		return dto.AuthStateFailed, fmt.Errorf("HH error: %s", msg)
	}
	if visible, _ := s.sessionPage.Locator("div[data-qa='login-error']").IsVisible(); visible {
		msg, _ := s.sessionPage.Locator("div[data-qa='login-error']").InnerText()
		return dto.AuthStateFailed, fmt.Errorf("HH login error: %s", msg)
	}

	if visible, _ := s.sessionPage.Locator("input[data-qa='magritte-pincode-input-field']").IsVisible(); visible {
		return dto.AuthStateWaitOTP, nil
	}
	if visible, _ := s.sessionPage.Locator("input[name='code']").IsVisible(); visible {
		return dto.AuthStateWaitOTP, nil
	}
	if visible, _ := s.sessionPage.Locator("img[data-qa='account-captcha-picture']").IsVisible(); visible {
		return dto.AuthStateWaitCaptcha, nil
	}
	if visible, _ := s.sessionPage.Locator("img[src*='captcha']").IsVisible(); visible {
		return dto.AuthStateWaitCaptcha, nil
	}

	return dto.AuthStateNone, nil
}

func (s *HHAuthService) handleIdentify(identify string) error {
	s.mu.Lock()
	s.status.State = dto.AuthStateWaitIdentify
	s.mu.Unlock()

	// Fill email/phone
	err := s.sessionPage.Fill("input[data-qa='login-input-username']", identify)
	if err != nil {
		_ = s.sessionPage.Fill("input[name='login']", identify)
	}

	err = s.sessionPage.Keyboard().Press("Enter")
	if err != nil {
		return fmt.Errorf("failed to submit login: %w", err)
	}

	return nil
}

func (s *HHAuthService) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.State != dto.AuthStateFailed && s.status.State != dto.AuthStateCompleted {
		s.status.State = dto.AuthStateFailed
		s.status.ErrorMessage = err.Error()
		logger.Error(logger.HH, "Auth failed: %v", err)
		s.stop()
	}
}

func (s *HHAuthService) complete(code string, userAgent string) {
	s.mu.Lock()
	if s.status.State == dto.AuthStateCompleted {
		s.mu.Unlock()
		return
	}
	s.status.State = dto.AuthStateWaitRedirect
	s.mu.Unlock()

	// Exchange token
	tr, err := ExchangeToken(code, userAgent)
	if err != nil {
		s.fail(err)
		return
	}

	// Save to repo
	now := time.Now()
	expiresAt := now.Add(time.Duration(tr.ExpiresIn) * time.Second)
	integration := &models.Integration{
		Platform:     "hh",
		Identifier:   s.identify,
		AccountID:    s.accountID,
		AccessToken:  &tr.AccessToken,
		RefreshToken: &tr.RefreshToken,
		ExpiresAt:    &expiresAt,
		UserAgent:    &userAgent,
		Status:       "active",
		CreatedAt:    now,
	}

	dbCtx, dbCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer dbCancel()
	err = s.repo.SaveIntegration(dbCtx, integration)
	if err != nil {
		s.fail(err)
		return
	}

	s.mu.Lock()
	s.status.State = dto.AuthStateCompleted
	s.mu.Unlock()
	s.stop()
}

func (s *HHAuthService) SubmitOTP(ctx context.Context, code string) (*dto.HHAuthStatus, error) {
	s.mu.Lock()
	if s.status.State != dto.AuthStateWaitOTP {
		s.mu.Unlock()
		return nil, fmt.Errorf("not in OTP state: %s", s.status.State)
	}
	s.mu.Unlock()

	select {
	case s.otpChan <- code:
		return s.GetStatus(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stopChan:
		return s.GetStatus(ctx)
	}
}

func (s *HHAuthService) SubmitCaptcha(ctx context.Context, resolution string) (*dto.HHAuthStatus, error) {
	s.mu.Lock()
	if s.status.State != dto.AuthStateWaitCaptcha {
		s.mu.Unlock()
		return nil, fmt.Errorf("not in Captcha state: %s", s.status.State)
	}
	s.mu.Unlock()

	select {
	case s.captchaChan <- resolution:
		return s.GetStatus(ctx)
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.stopChan:
		return s.GetStatus(ctx)
	}
}

func (s *HHAuthService) GetStatus(ctx context.Context) (*dto.HHAuthStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}

func (s *HHAuthService) FetchNegotiations(ctx context.Context, accountID string) ([]dto.HHNegotiation, error) {
	logger.Debug(logger.HH, "Fetching negotiations for account %s", accountID)
	req, _ := http.NewRequestWithContext(ctx, "GET", HHApiURL+"negotiations", nil)
	_ = req
	return []dto.HHNegotiation{}, nil
}
