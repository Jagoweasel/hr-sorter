package repository

import (
	"context"
	"hr-sorter/internal/models"
	"time"

	"github.com/jmoiron/sqlx"
)

type IntegrationRepository struct {
	db *sqlx.DB
}

func NewIntegrationRepository(db *sqlx.DB) *IntegrationRepository {
	return &IntegrationRepository{db: db}
}

func (r *IntegrationRepository) GetByID(ctx context.Context, id interface{}) (*models.Integration, error) {
	var i models.Integration
	err := r.db.GetContext(ctx, &i, "SELECT * FROM integrations WHERE id = ?", id)
	return &i, err
}

func (r *IntegrationRepository) GetByAccountID(ctx context.Context, accountID interface{}) ([]models.Integration, error) {
	var ints []models.Integration
	err := r.db.SelectContext(ctx, &ints, "SELECT * FROM integrations WHERE account_id = ?", accountID)
	return ints, err
}

func (r *IntegrationRepository) Create(ctx context.Context, accID, platform, identifier string, apiID int, apiHash, status, sessionPath string, userAgent *string) (int64, error) {
	res, err := r.db.ExecContext(ctx, "INSERT INTO integrations (account_id, platform, identifier, api_id, api_hash, status, session_path, user_agent) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		accID, platform, identifier, apiID, apiHash, status, sessionPath, userAgent)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *IntegrationRepository) UpdateStatus(ctx context.Context, id interface{}, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE integrations SET status = ? WHERE id = ?", status, id)
	return err
}

func (r *IntegrationRepository) UpdateTokens(ctx context.Context, id interface{}, accessToken, refreshToken string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE integrations SET access_token = ?, refresh_token = ?, expires_at = ?, status = 'active' WHERE id = ?",
		accessToken, refreshToken, expiresAt, id)
	return err
}

func (r *IntegrationRepository) Delete(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM integrations WHERE id = ?", id)
	return err
}

func (r *IntegrationRepository) GetActiveAndPending(ctx context.Context) ([]models.Integration, error) {
	var integrations []models.Integration
	err := r.db.SelectContext(ctx, &integrations, `
		SELECT i.* FROM integrations i
		JOIN accounts a ON i.account_id = a.id
		WHERE i.status IN ('active', 'pending_auth') AND a.status = 'active'
	`)
	return integrations, err
}
