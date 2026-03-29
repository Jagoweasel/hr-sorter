package repository

import (
	"context"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type FilterRepository struct {
	db *sqlx.DB
}

func NewFilterRepository(db *sqlx.DB) *FilterRepository {
	return &FilterRepository{db: db}
}

func (r *FilterRepository) GetAll(ctx context.Context) ([]models.MessageFilter, error) {
	var filters []models.MessageFilter
	err := r.db.SelectContext(ctx, &filters, "SELECT * FROM message_filters ORDER BY created_at DESC")
	return filters, err
}

func (r *FilterRepository) GetActivePatterns(ctx context.Context) ([]string, error) {
	var patterns []string
	err := r.db.SelectContext(ctx, &patterns, "SELECT pattern FROM message_filters WHERE is_active = 1")
	return patterns, err
}

func (r *FilterRepository) Create(ctx context.Context, pattern string) error {
	logger.Debug(logger.Filters, "Repo: Inserting pattern '%s'", pattern)
	res, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO message_filters (pattern) VALUES (?)", pattern)
	if err != nil {
		logger.Debug(logger.Filters, "Repo: Error inserting pattern '%s': %v", pattern, err)
		return err
	}
	rows, _ := res.RowsAffected()
	logger.Debug(logger.Filters, "Repo: Pattern '%s' inserted, rows affected: %d", pattern, rows)
	return nil
}

func (r *FilterRepository) Delete(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM message_filters WHERE id = ?", id)
	return err
}

func (r *FilterRepository) Toggle(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "UPDATE message_filters SET is_active = NOT is_active WHERE id = ?", id)
	return err
}

func (r *FilterRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM message_filters")
	return err
}
