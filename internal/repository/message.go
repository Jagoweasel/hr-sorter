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
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		integrationID, contactID, externalID, text, isIncoming, timestamp)
	return err
}
