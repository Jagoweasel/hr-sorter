package domain

import (
	"context"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/models"
	"io"
)

// HHClient interface for HeadHunter integration
type HHClient interface {
	// StartAuth begins the OAuth flow with Playwright-go
	StartAuth(ctx context.Context, identify string) (*dto.HHAuthStatus, error)
	// SubmitOTP enters the received code into the flow
	SubmitOTP(ctx context.Context, code string) (*dto.HHAuthStatus, error)
	// SubmitCaptcha provides resolution for an HH-requested captcha
	SubmitCaptcha(ctx context.Context, resolution string) (*dto.HHAuthStatus, error)
	// GetStatus returns the current authentication state machine status
	GetStatus(ctx context.Context) (*dto.HHAuthStatus, error)
	// FetchNegotiations retrieves job applications from HH API
	FetchNegotiations(ctx context.Context, accountID string) ([]dto.HHNegotiation, error)
	// Close shuts down the Playwright instance
	Close() error
}

// Reporter interface for PDF generation
type Reporter interface {
	// GeneratePDF generates a professional-grade PDF using maroto/v2
	// Uses embedded fonts and grid layout.
	GeneratePDF(ctx context.Context, data *models.ReportData) ([]byte, error)
}

// Translator interface for i18n
type Translator interface {
	// Translate converts a key to its localized string based on locale
	Translate(key string, locale string, args ...interface{}) string
}

// VacancyMapper interface for categorization logic
type VacancyMapper interface {
	// Classify categorizes a vacancy title into a predefined type (e.g., Lead, Developer)
	Classify(title string) string
	// UpdateRules refreshes the regex mapping rules from storage
	UpdateRules(ctx context.Context, rules map[string]string) error
}

// LogStreamer interface for real-time log broadcasting
type LogStreamer interface {
	// Stream starts broadcasting zap logger output to connected clients (WS/SSE)
	Stream(ctx context.Context, writer io.Writer) error
	// Broadcast sends a log entry to all active subscribers
	Broadcast(entry string)
}

// Repository interface for persistence
type Repository interface {
	SaveAccount(ctx context.Context, account *models.Account) error
	GetAccount(ctx context.Context, id int64) (*models.Account, error)
	SaveNegotiations(ctx context.Context, negotiations []dto.HHNegotiation) error
	SaveNegotiationsStats(ctx context.Context, stats *models.NegotiationStats) error
	GetMappingRules(ctx context.Context) (map[string]string, error)

	// Caching support
	GetMessageFilters(ctx context.Context) ([]models.MessageFilter, error)
	SaveMessageFilter(ctx context.Context, filter *models.MessageFilter) error

	GetIntegration(ctx context.Context, id int64) (*models.Integration, error)
	SaveIntegration(ctx context.Context, integration *models.Integration) error

	GetSequence(ctx context.Context, id int64) (*models.Sequence, error)
	SaveSequence(ctx context.Context, sequence *models.Sequence) error
}
