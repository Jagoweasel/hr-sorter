package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type MessageRepository struct {
	db *sqlx.DB
}

func NewMessageRepository(db *sqlx.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) GetByContactID(ctx context.Context, contactID interface{}) ([]models.Message, error) {
	var messages []models.Message
	err := r.db.SelectContext(ctx, &messages, "SELECT * FROM messages WHERE contact_id = ? ORDER BY timestamp ASC", contactID)
	return messages, err
}

func (r *MessageRepository) Create(ctx context.Context, integrationID, contactID int64, externalID, text string, isIncoming bool, timestamp string) error {
	return r.CreateExt(ctx, r.db, integrationID, contactID, externalID, text, isIncoming, timestamp)
}

func (r *MessageRepository) CreateExt(ctx context.Context, ext sqlx.ExtContext, integrationID, contactID int64, externalID, text string, isIncoming bool, timestamp string) error {
	_, err := ext.ExecContext(ctx, "INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		integrationID, contactID, externalID, text, isIncoming, timestamp)
	return err
}

func (r *MessageRepository) GetLastMessageTimeByContactID(ctx context.Context, contactID int64) (string, error) {
	var ts string
	err := r.db.GetContext(ctx, &ts, "SELECT COALESCE(MAX(timestamp), '1970-01-01 00:00:00') FROM messages WHERE contact_id = ?", contactID)
	return ts, err
}
