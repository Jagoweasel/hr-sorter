package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type AccountWithIntegrations struct {
	models.Account
	Integrations []models.Integration
}

type AccountRepository struct {
	db *sqlx.DB
}

func NewAccountRepository(db *sqlx.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) GetAll(ctx context.Context) ([]models.Account, error) {
	var accounts []models.Account
	err := r.db.SelectContext(ctx, &accounts, "SELECT * FROM accounts ORDER BY name")
	return accounts, err
}

func (r *AccountRepository) GetAllWithIntegrations(ctx context.Context) ([]AccountWithIntegrations, error) {
	var accounts []models.Account
	err := r.db.SelectContext(ctx, &accounts, "SELECT * FROM accounts ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}

	var allIntegrations []models.Integration
	err = r.db.SelectContext(ctx, &allIntegrations, "SELECT * FROM integrations")
	if err != nil {
		return nil, err
	}

	// Map integrations to accounts
	intsByAccID := make(map[int64][]models.Integration)
	for _, i := range allIntegrations {
		intsByAccID[i.AccountID] = append(intsByAccID[i.AccountID], i)
	}

	var data []AccountWithIntegrations
	for _, acc := range accounts {
		data = append(data, AccountWithIntegrations{
			Account:      acc,
			Integrations: intsByAccID[acc.ID],
		})
	}

	return data, nil
}

func (r *AccountRepository) GetActive(ctx context.Context) ([]models.Account, error) {
	var accounts []models.Account
	err := r.db.SelectContext(ctx, &accounts, "SELECT * FROM accounts WHERE status = 'active' ORDER BY name")
	return accounts, err
}

func (r *AccountRepository) GetByID(ctx context.Context, id interface{}) (*models.Account, error) {
	var acc models.Account
	err := r.db.GetContext(ctx, &acc, "SELECT * FROM accounts WHERE id = ?", id)
	return &acc, err
}

func (r *AccountRepository) GetStatus(ctx context.Context, id interface{}) (string, error) {
	var status string
	err := r.db.GetContext(ctx, &status, "SELECT status FROM accounts WHERE id = ?", id)
	return status, err
}

func (r *AccountRepository) Create(ctx context.Context, name, slug string) error {
	_, err := r.db.ExecContext(ctx, "INSERT INTO accounts (name, slug, status) VALUES (?, ?, 'active')", name, slug)
	return err
}

func (r *AccountRepository) UpdateStatus(ctx context.Context, id interface{}, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE accounts SET status = ? WHERE id = ?", status, id)
	return err
}

func (r *AccountRepository) Update(ctx context.Context, id interface{}, name, slug string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE accounts SET name = ?, slug = ? WHERE id = ?", name, slug, id)
	return err
}

func (r *AccountRepository) Delete(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)
	return err
}
