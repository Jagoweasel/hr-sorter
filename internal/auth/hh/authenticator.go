package hh

import (
	"context"
	"encoding/json"
	"fmt"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/logger"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Authenticator defines the high-level logic for HeadHunter authorization.
// It manages multiple authentication flows and handles token acquisition.
type Authenticator interface {
	// StartFlow starts a new authentication process for a specific account.
	// It uses Playwright to navigate the HH login page and perform identification.
	StartFlow(ctx context.Context, accountID int64, identifier string) (AuthFlow, error)

	// GetFlow retrieves an existing authentication flow by account identifier.
	GetFlow(accountID int64) (AuthFlow, bool)

	// Close stops playwright and closes the browser.
	Close() error
}

// AuthFlow represents an active authentication process.
// It encapsulates Playwright browser context and session-specific logic.
type AuthFlow interface {
	// GetStatus returns the current progress of the authentication.
	GetStatus() *dto.HHAuthStatus

	// SubmitOTP sends the one-time password (OTP) to the login process.
	SubmitOTP(code string) error

	// SubmitCaptcha sends the solved captcha text to the login process.
	SubmitCaptcha(solution string) error

	// SubmitCode manually provides the OAuth authorization code.
	SubmitCode(code string) error

	// Cancel terminates the flow prematurely.
	Cancel() error

	// Done returns a channel that is closed when the flow is finished (success or failure).
	Done() <-chan struct{}

	// Result returns the session if the flow completed successfully.
	Result() (*Session, error)
}

// Session represents a fully authenticated HH session for an integration.
type Session struct {
	mu           sync.RWMutex
	AccountID    int64
	Identifier   string // email or phone
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	UserAgent    string
}

func (s *Session) GetTokens() (string, string, time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.AccessToken, s.RefreshToken, s.ExpiresAt
}

func (s *Session) SetTokens(access, refresh string, expires time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AccessToken = access
	s.RefreshToken = refresh
	s.ExpiresAt = expires
}

func (s *Session) GetUserAgent() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.UserAgent
}

type PlaywrightAuthenticator struct {
	pw       *playwright.Playwright
	browser  playwright.Browser
	flows    map[int64]*PlaywrightAuthFlow
	mu       sync.RWMutex
	storage  SessionStorage
	config   ClientConfig
	uaGen    UserAgentGenerator
	headless bool
}

func NewPlaywrightAuthenticator(storage SessionStorage, config ClientConfig, uaGen UserAgentGenerator, headless bool) (*PlaywrightAuthenticator, error) {
	logger.Info(logger.HH, "[HHAuth] Initializing Playwright module...")
	pw, err := playwright.Run()
	if err != nil {
		logger.Error(logger.HH, "[HHAuth] Failed to start Playwright: %v", err)
		return nil, fmt.Errorf("failed to start playwright: %w", err)
	}

	logger.Debug(logger.HH, "[HHAuth] Launching Chromium (headless=%v)...", headless)
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
		Args: []string{
			"--no-sandbox",
			"--disable-setuid-sandbox",
		},
	})
	if err != nil {
		logger.Error(logger.HH, "[HHAuth] Failed to launch Chromium: %v", err)
		_ = pw.Stop()
		return nil, fmt.Errorf("failed to launch browser: %w", err)
	}

	logger.Info(logger.HH, "[HHAuth] Authenticator initialized successfully (Playwright version: %s)", "v0.5700.1")
	return &PlaywrightAuthenticator{
		pw:       pw,
		browser:  browser,
		flows:    make(map[int64]*PlaywrightAuthFlow),
		storage:  storage,
		config:   config,
		uaGen:    uaGen,
		headless: headless,
	}, nil
}

func (a *PlaywrightAuthenticator) StartFlow(ctx context.Context, accountID int64, identifier string) (AuthFlow, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if flow, ok := a.flows[accountID]; ok {
		return flow, nil
	}

	flow := &PlaywrightAuthFlow{
		accountID:  accountID,
		identifier: identifier,
		status:     &dto.HHAuthStatus{State: dto.AuthStateWaitIdentify},
		done:       make(chan struct{}),
		otpChan:    make(chan string, 1),
		capChan:    make(chan string, 1),
		codeChan:   make(chan string, 1),
		stopChan:   make(chan struct{}),
		storage:    a.storage,
		config:     a.config,
		userAgent:  a.uaGen.Generate(),
		browser:    a.browser,
		pw:         a.pw,
		onComplete: func() {
			a.mu.Lock()
			delete(a.flows, accountID)
			a.mu.Unlock()
		},
	}

	a.flows[accountID] = flow
	go flow.run()

	return flow, nil
}

func (a *PlaywrightAuthenticator) GetFlow(accountID int64) (AuthFlow, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	flow, ok := a.flows[accountID]
	return flow, ok
}

func (a *PlaywrightAuthenticator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	for _, flow := range a.flows {
		_ = flow.Cancel()
	}

	var errs []string
	if a.browser != nil {
		if err := a.browser.Close(); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if a.pw != nil {
		if err := a.pw.Stop(); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing authenticator: %s", strings.Join(errs, "; "))
	}
	return nil
}

type PlaywrightAuthFlow struct {
	accountID  int64
	identifier string
	status     *dto.HHAuthStatus
	mu         sync.RWMutex
	done       chan struct{}
	otpChan    chan string
	capChan    chan string
	codeChan   chan string
	stopChan   chan struct{}
	storage    SessionStorage
	config     ClientConfig
	userAgent  string
	browser    playwright.Browser
	pw         *playwright.Playwright
	session    *Session
	err        error
	onComplete func()

	// Playwright objects
	context playwright.BrowserContext
	page    playwright.Page
}

func (f *PlaywrightAuthFlow) GetStatus() *dto.HHAuthStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.status
}

func (f *PlaywrightAuthFlow) SubmitOTP(code string) error {
	select {
	case f.otpChan <- code:
		return nil
	case <-f.done:
		return fmt.Errorf("flow already finished")
	}
}

func (f *PlaywrightAuthFlow) SubmitCaptcha(solution string) error {
	select {
	case f.capChan <- solution:
		return nil
	case <-f.done:
		return fmt.Errorf("flow already finished")
	}
}

func (f *PlaywrightAuthFlow) SubmitCode(code string) error {
	select {
	case f.codeChan <- code:
		return nil
	case <-f.done:
		return fmt.Errorf("flow already finished")
	}
}

func (f *PlaywrightAuthFlow) Cancel() error {
	select {
	case <-f.stopChan:
		return nil
	default:
		close(f.stopChan)
		return nil
	}
}

func (f *PlaywrightAuthFlow) Done() <-chan struct{} {
	return f.done
}

func (f *PlaywrightAuthFlow) Result() (*Session, error) {
	<-f.done
	return f.session, f.err
}

func (f *PlaywrightAuthFlow) run() {
	logger.Info(logger.HH, "[HHAuth] Starting flow for Account %d (%s)", f.accountID, f.identifier)
	defer close(f.done)
	defer func() {
		if f.onComplete != nil {
			f.onComplete()
		}
		if f.page != nil {
			f.page.Close()
		}
		if f.context != nil {
			f.context.Close()
		}
		logger.Info(logger.HH, "[HHAuth] Flow finished for Account %d", f.accountID)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var err error
	// Set up browser context with mobile emulation and correct User-Agent
	device := f.pw.Devices["Pixel 7"]
	f.context, err = f.browser.NewContext(playwright.BrowserNewContextOptions{
		UserAgent:         playwright.String(f.userAgent), // Correct: use our generated UA
		Viewport:          device.Viewport,
		DeviceScaleFactor: playwright.Float(device.DeviceScaleFactor),
		IsMobile:          playwright.Bool(device.IsMobile),
		HasTouch:          playwright.Bool(device.HasTouch),
	})
	if err != nil {
		f.fail(fmt.Errorf("failed to create context: %w", err))
		return
	}

	f.page, err = f.context.NewPage()
	if err != nil {
		f.fail(fmt.Errorf("failed to create page: %w", err))
		return
	}

	// Request Interception for hhandroid://
	f.context.OnRequest(func(request playwright.Request) {
		logger.Trace(logger.HH, "[HHAuth][Network] Request: %s", request.URL())
		if strings.HasPrefix(request.URL(), "hhandroid://") {
			redactedURL := request.URL()
			if u, err := url.Parse(redactedURL); err == nil {
				q := u.Query()
				if q.Get("code") != "" {
					q.Set("code", "[REDACTED]")
					u.RawQuery = q.Encode()
					redactedURL = u.String()
				}
			}
			logger.Info(logger.HH, "[HHAuth] Detected redirect: %s", redactedURL)

			u, _ := url.Parse(request.URL())
			code := u.Query().Get("code")
			if code != "" {
				logger.Debug(logger.HH, "[HHAuth] Extracted code from redirect URL")
				select {
				case f.codeChan <- code:
				default:
				}
			}
		}
	})

	// Get authorize URL (adapted from ExchangeToken logic)
	authURL := fmt.Sprintf("https://hh.ru/oauth/authorize?response_type=code&client_id=%s&state=skip", f.config.ClientID)

	_, err = f.page.Goto(authURL, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateLoad,
	})
	if err != nil {
		f.fail(fmt.Errorf("failed to navigate to auth URL: %w", err))
		return
	}

	// Identification
	if err := f.handleIdentify(); err != nil {
		f.fail(err)
		return
	}

	for {
		select {
		case <-f.stopChan:
			f.fail(fmt.Errorf("cancelled by user"))
			return
		case <-ctx.Done():
			f.fail(fmt.Errorf("timeout"))
			return
		case code := <-f.codeChan:
			f.complete(code)
			return
		default:
			state, err := f.detectState()
			if err != nil {
				f.fail(err)
				return
			}

			switch state {
			case dto.AuthStateWaitOTP:
				f.setStatus(dto.AuthStateWaitOTP, "")
				select {
				case code := <-f.otpChan:
					if err := f.page.Fill("input[data-qa='magritte-pincode-input-field']", code); err != nil {
						_ = f.page.Fill("input[name='code']", code)
					}
					_ = f.page.Keyboard().Press("Enter")
				case <-f.stopChan:
					return
				case <-ctx.Done():
					return
				}
			case dto.AuthStateWaitCaptcha:
				imgElement := f.page.Locator("img[data-qa='account-captcha-picture']")
				if visible, _ := imgElement.IsVisible(); !visible {
					imgElement = f.page.Locator("img[src*='captcha']")
				}
				screenshot, _ := imgElement.Screenshot()
				f.mu.Lock()
				f.status.State = dto.AuthStateWaitCaptcha
				f.status.CaptchaImg = screenshot
				f.mu.Unlock()

				select {
				case solution := <-f.capChan:
					if err := f.page.Fill("input[data-qa='account-captcha-input']", solution); err != nil {
						_ = f.page.Fill("input[name='captcha']", solution)
					}
					_ = f.page.Keyboard().Press("Enter")
				case <-f.stopChan:
					return
				case <-ctx.Done():
					return
				}
			case dto.AuthStateFailed:
				// Already handled in detectState
				return
			default:
				time.Sleep(2 * time.Second)
			}
		}
	}
}

func (f *PlaywrightAuthFlow) handleIdentify() error {
	logger.Debug(logger.HH, "[HHAuth] Identifying user: %s", f.identifier)
	f.setStatus(dto.AuthStateWaitIdentify, "")

	// Wait for input
	_, err := f.page.WaitForSelector("input[data-qa='login-input-username'], input[name='login']", playwright.PageWaitForSelectorOptions{
		Timeout: playwright.Float(10000),
	})
	if err != nil {
		logger.Error(logger.HH, "[HHAuth] Login input not found on page: %s", f.page.URL())
		return fmt.Errorf("login input not found: %w", err)
	}

	logger.Trace(logger.HH, "[HHAuth] Filling login input...")
	err = f.page.Fill("input[data-qa='login-input-username']", f.identifier)
	if err != nil {
		_ = f.page.Fill("input[name='login']", f.identifier)
	}
	logger.Trace(logger.HH, "[HHAuth] Pressing Enter...")
	return f.page.Keyboard().Press("Enter")
}

func (f *PlaywrightAuthFlow) detectState() (dto.HHAuthState, error) {
	logger.Trace(logger.HH, "[HHAuth] Detecting state for page: %s", f.page.URL())
	// Check for errors first
	if visible, _ := f.page.Locator("div.bloko-notification--error, div[data-qa='login-error']").IsVisible(); visible {
		msg, _ := f.page.Locator("div.bloko-notification--error, div[data-qa='login-error']").InnerText()
		logger.Error(logger.HH, "[HHAuth] Detected HH error: %s", msg)
		return dto.AuthStateFailed, fmt.Errorf("HH error: %s", msg)
	}

	if visible, _ := f.page.Locator("input[data-qa='magritte-pincode-input-field'], input[name='code']").IsVisible(); visible {
		logger.Debug(logger.HH, "[HHAuth] Detected OTP state")
		return dto.AuthStateWaitOTP, nil
	}
	if visible, _ := f.page.Locator("img[data-qa='account-captcha-picture'], img[src*='captcha']").IsVisible(); visible {
		logger.Debug(logger.HH, "[HHAuth] Detected Captcha state")
		return dto.AuthStateWaitCaptcha, nil
	}

	return dto.AuthStateNone, nil
}

func (f *PlaywrightAuthFlow) fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
	f.status.State = dto.AuthStateFailed
	f.status.ErrorMessage = err.Error()
	logger.Error(logger.HH, "Auth flow failed for %d: %v", f.accountID, err)
}

func (f *PlaywrightAuthFlow) setStatus(state dto.HHAuthState, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status.State = state
	f.status.ErrorMessage = msg
}

func (f *PlaywrightAuthFlow) complete(code string) {
	f.setStatus(dto.AuthStateWaitRedirect, "Exchanging token...")

	// Exchange token using the common logic (I'll implement it in client.go or similar)
	tr, err := f.exchangeToken(code)
	if err != nil {
		f.fail(err)
		return
	}

	f.session = &Session{
		AccountID:    f.accountID,
		Identifier:   f.identifier,
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
		UserAgent:    f.userAgent,
	}

	if err := f.storage.Save(context.Background(), f.session); err != nil {
		f.fail(fmt.Errorf("failed to save session: %w", err))
		return
	}

	f.setStatus(dto.AuthStateCompleted, "")
}

func (f *PlaywrightAuthFlow) exchangeToken(code string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("client_id", f.config.ClientID)
	data.Set("client_secret", f.config.ClientSecret)

	req, err := http.NewRequest("POST", "https://hh.ru/oauth/token", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(bodyBytes))
	}

	var tr TokenResponse
	if err := json.Unmarshal(bodyBytes, &tr); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}
	return &tr, nil
}
