package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type ContactRepository struct {
	db *sqlx.DB
}

type ContactWithLastMsg struct {
	models.Contact
	LastMessage    string `db:"last_message"`
	LastTime       string `db:"last_time"`
	LastIsIncoming bool   `db:"last_is_incoming"`
	MsgCount       int    `db:"msg_count"`
}

func NewContactRepository(db *sqlx.DB) *ContactRepository {
	return &ContactRepository{db: db}
}

func (r *ContactRepository) GetByID(ctx context.Context, id interface{}) (*models.Contact, error) {
	var c models.Contact
	err := r.db.GetContext(ctx, &c, "SELECT * FROM contacts WHERE id = ?", id)
	return &c, err
}

func (r *ContactRepository) GetAll(ctx context.Context, accountID, platform string, showDeclines bool) ([]ContactWithLastMsg, error) {
	query := `
		SELECT c.*, 
		       COALESCE((SELECT text FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), '') as last_message,
			   COALESCE((SELECT datetime(timestamp) FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), datetime(c.created_at)) as last_time,
			   COALESCE((SELECT is_incoming FROM messages WHERE contact_id = c.id ORDER BY timestamp DESC LIMIT 1), 0) as last_is_incoming,
			   (SELECT COUNT(*) FROM messages WHERE contact_id = c.id) as msg_count,
			   EXISTS(SELECT 1 FROM sequence_contacts WHERE contact_id = c.id) as in_sequence,
			   COALESCE((SELECT s.status FROM sequences s JOIN sequence_contacts sc ON s.id = sc.sequence_id WHERE sc.contact_id = c.id LIMIT 1), '') as seq_status
		FROM contacts c
		JOIN integrations i ON c.integration_id = i.id
		WHERE 1=1`

	var args []interface{}
	if accountID != "" {
		query += " AND i.account_id = ?"
		args = append(args, accountID)
	}
	if platform != "" {
		query += " AND c.platform = ?"
		args = append(args, platform)
	}
	if !showDeclines {
		query += " AND NOT (c.platform = 'hh' AND (c.username = 'Отказ' OR c.username = 'discard'))"
	}
	query += " ORDER BY last_time DESC"

	var contacts []ContactWithLastMsg
	err := r.db.SelectContext(ctx, &contacts, query, args...)
	return contacts, err
}

func (r *ContactRepository) UpsertTGContact(ctx context.Context, integrationID int64, externalID, firstName, lastName, username string, accessHash int64) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contacts (integration_id, platform, external_id, first_name, last_name, username, access_hash) 
		VALUES (?, 'tg', ?, ?, ?, ?, ?)
		ON CONFLICT(external_id) DO UPDATE SET 
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			username = excluded.username,
			access_hash = CASE WHEN excluded.access_hash != 0 THEN excluded.access_hash ELSE access_hash END
	`, integrationID, externalID, firstName, lastName, username, accessHash)
	return err
}

func (r *ContactRepository) GetIDByExternalID(ctx context.Context, platform, externalID string) (int64, error) {
	var id int64
	err := r.db.GetContext(ctx, &id, "SELECT id FROM contacts WHERE platform = ? AND external_id = ?", platform, externalID)
	return id, err
}

func (r *ContactRepository) GetAccountIDByContactID(ctx context.Context, contactID interface{}) (*int64, error) {
	var accountID int64
	err := r.db.GetContext(ctx, &accountID, `
		SELECT i.account_id FROM contacts c 
		JOIN integrations i ON c.integration_id = i.id 
		WHERE c.id = ? AND i.account_id IS NOT NULL LIMIT 1
	`, contactID)
	if err != nil {
		return nil, err
	}
	return &accountID, nil
}
