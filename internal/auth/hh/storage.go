package hh

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// SessionStorage defines the interface for persisting HeadHunter sessions.
// It allows retrieving sessions across application restarts.
type SessionStorage interface {
	// Save stores a HeadHunter session for a specific account.
	// It should overwrite any existing session for the same AccountID.
	Save(ctx context.Context, session *Session) error

	// Get retrieves a stored session for a specific account.
	// Returns nil and no error if the session is not found.
	Get(ctx context.Context, accountID int64) (*Session, error)

	// Delete removes a stored session for a specific account.
	// Delete removals a stored session for a specific account.
	Delete(ctx context.Context, accountID int64) error

	// ListAll retrieves all stored sessions from the storage.
	ListAll(ctx context.Context) ([]*Session, error)
}

type SQLiteSessionStorage struct {
	db *sqlx.DB
}

func NewSQLiteSessionStorage(db *sqlx.DB) SessionStorage {
	return &SQLiteSessionStorage{db: db}
}

func (s *SQLiteSessionStorage) Save(ctx context.Context, session *Session) error {
	query := `
		INSERT INTO integrations (account_id, platform, identifier, access_token, refresh_token, expires_at, user_agent, status, created_at)
		VALUES (?, 'hh', ?, ?, ?, ?, ?, 'active', CURRENT_TIMESTAMP)
		ON CONFLICT(account_id, platform) DO UPDATE SET
			identifier = EXCLUDED.identifier,
			access_token = EXCLUDED.access_token,
			refresh_token = EXCLUDED.refresh_token,
			expires_at = EXCLUDED.expires_at,
			user_agent = EXCLUDED.user_agent,
			status = 'active'
	`
	_, err := s.db.ExecContext(ctx, query,
		session.AccountID,
		session.Identifier,
		session.AccessToken,
		session.RefreshToken,
		session.ExpiresAt,
		session.UserAgent,
	)
	if err != nil {
		return fmt.Errorf("failed to save hh session: %w", err)
	}
	return nil
}

func (s *SQLiteSessionStorage) Get(ctx context.Context, accountID int64) (*Session, error) {
	query := `
		SELECT account_id, identifier, access_token, refresh_token, expires_at, user_agent
		FROM integrations
		WHERE account_id = ? AND platform = 'hh'
	`
	var row struct {
		AccountID    int64     `db:"account_id"`
		Identifier   string    `db:"identifier"`
		AccessToken  string    `db:"access_token"`
		RefreshToken string    `db:"refresh_token"`
		ExpiresAt    time.Time `db:"expires_at"`
		UserAgent    string    `db:"user_agent"`
	}

	err := s.db.GetContext(ctx, &row, query, accountID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get hh session: %w", err)
	}

	return &Session{
		AccountID:    row.AccountID,
		Identifier:   row.Identifier,
		AccessToken:  row.AccessToken,
		RefreshToken: row.RefreshToken,
		ExpiresAt:    row.ExpiresAt,
		UserAgent:    row.UserAgent,
	}, nil
}

func (s *SQLiteSessionStorage) Delete(ctx context.Context, accountID int64) error {
	query := `DELETE FROM integrations WHERE account_id = ? AND platform = 'hh'`
	_, err := s.db.ExecContext(ctx, query, accountID)
	if err != nil {
		return fmt.Errorf("failed to delete hh session: %w", err)
	}
	return nil
}

func (s *SQLiteSessionStorage) ListAll(ctx context.Context) ([]*Session, error) {
	query := `
		SELECT account_id, identifier, access_token, refresh_token, expires_at, user_agent
		FROM integrations
		WHERE platform = 'hh'
	`
	var rows []struct {
		AccountID    int64     `db:"account_id"`
		Identifier   string    `db:"identifier"`
		AccessToken  string    `db:"access_token"`
		RefreshToken string    `db:"refresh_token"`
		ExpiresAt    time.Time `db:"expires_at"`
		UserAgent    string    `db:"user_agent"`
	}

	err := s.db.SelectContext(ctx, &rows, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list hh sessions: %w", err)
	}

	sessions := make([]*Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, &Session{
			AccountID:    row.AccountID,
			Identifier:   row.Identifier,
			AccessToken:  row.AccessToken,
			RefreshToken: row.RefreshToken,
			ExpiresAt:    row.ExpiresAt,
			UserAgent:    row.UserAgent,
		})
	}
	return sessions, nil
}
