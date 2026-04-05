package models

import "time"

type Account struct {
	ID        int64     `db:"id" json:"id"`
	Name      string    `db:"name" json:"name"`
	Slug      *string   `db:"slug" json:"slug"`
	Status    string    `db:"status" json:"status"` // "active", "inactive"
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Integration struct {
	ID           int64      `db:"id" json:"id"`
	AccountID    int64      `db:"account_id" json:"account_id"`
	Platform     string     `db:"platform" json:"platform"` // "tg", "hh"
	Identifier   string     `db:"identifier" json:"identifier"`
	APIID        int        `db:"api_id" json:"api_id"`
	APIHash      string     `db:"api_hash" json:"api_hash"`
	AccessToken  *string    `db:"access_token" json:"access_token"`
	RefreshToken *string    `db:"refresh_token" json:"refresh_token"`
	ExpiresAt    *time.Time `db:"expires_at" json:"expires_at"`
	UserAgent    *string    `db:"user_agent" json:"user_agent"`
	Status       string     `db:"status" json:"status"` // "active", "pending_auth", "inactive"
	SessionPath  string     `db:"session_path" json:"session_path"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
}

type Contact struct {
	ID            int64     `db:"id" json:"id"`
	IntegrationID int64     `db:"integration_id" json:"integration_id"`
	Platform      string    `db:"platform" json:"platform"` // "tg", "hh"
	ExternalID    string    `db:"external_id" json:"external_id"`
	FirstName     *string   `db:"first_name" json:"first_name"`
	LastName      *string   `db:"last_name" json:"last_name"`
	Username      *string   `db:"username" json:"username"`
	AccessHash    int64     `db:"access_hash" json:"access_hash"`
	IsIgnored     bool      `db:"is_ignored" json:"is_ignored"`
	CreatedAt     time.Time `db:"created_at" json:"created_at"`
	// UI fields
	InSequence bool   `db:"in_sequence" json:"in_sequence"`
	SeqStatus  string `db:"seq_status" json:"seq_status"`
}

type Message struct {
	ID            int64   `db:"id" json:"id"`
	IntegrationID int64   `db:"integration_id" json:"integration_id"`
	ContactID     int64   `db:"contact_id" json:"contact_id"`
	ExternalID    *string `db:"external_id" json:"external_id"`
	Text          string  `db:"text" json:"text"`
	IsIncoming    bool    `db:"is_incoming" json:"is_incoming"`
	Timestamp     string  `db:"timestamp" json:"timestamp"`
}

type Sequence struct {
	ID              int64     `db:"id" json:"id"`
	AccountID       *int64    `db:"account_id" json:"account_id"`
	CompanyName     string    `db:"company_name" json:"company_name"`
	VacancyName     string    `db:"vacancy_name" json:"vacancy_name"`
	VacancyLink     *string   `db:"vacancy_link" json:"vacancy_link"`
	Category        *string   `db:"category" json:"category"`
	Status          string    `db:"status" json:"status"` // initial, screening, tech, final, offer, accepted, rejected
	RejectionReason *string   `db:"rejection_reason" json:"rejection_reason"`
	SummaryComment  *string   `db:"summary_comment" json:"summary_comment"`
	CreatedAt       time.Time `db:"created_at" json:"created_at"`
}

type MappingRule struct {
	ID       int64  `db:"id" json:"id"`
	Pattern  string `db:"pattern" json:"pattern"`
	Category string `db:"category" json:"category"`
}

type NegotiationStats struct {
	SequenceID        int64     `db:"sequence_id" json:"sequence_id"`
	ApplicationsCount int       `db:"applications_count" json:"applications_count"`
	UpdatedAt         time.Time `db:"updated_at" json:"updated_at"`
}

type InterviewStage struct {
	ID          int64      `db:"id" json:"id"`
	SequenceID  int64      `db:"sequence_id" json:"sequence_id"`
	Name        string     `db:"name" json:"name"`
	StageType   *string    `db:"stage_type" json:"stage_type"` // Theory, Live Coding, System Design
	ScheduledAt *time.Time `db:"scheduled_at" json:"scheduled_at"`
	Notes       *string    `db:"notes" json:"notes"`
	IsCompleted bool       `db:"is_completed" json:"is_completed"`
	OrderIndex  int        `db:"order_index" json:"order_index"`
}

type MessageFilter struct {
	ID        int64     `db:"id" json:"id"`
	Pattern   string    `db:"pattern" json:"pattern"`
	IsActive  bool      `db:"is_active" json:"is_active"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
