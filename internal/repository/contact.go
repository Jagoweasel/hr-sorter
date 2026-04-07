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
	AccountID      *int64 `db:"account_id"`
	LastMessage    string `db:"last_message"`
	LastTime       string `db:"last_time"`
	LastIsIncoming bool   `db:"last_is_incoming"`
	MsgCount       int    `db:"msg_count"`
	IsFiltered     bool   `db:"-"`
	SequenceIDs    string `db:"sequence_ids"`
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
		WITH LatestMessages AS (
			SELECT contact_id, text, timestamp, is_incoming,
				   ROW_NUMBER() OVER (PARTITION BY contact_id ORDER BY timestamp DESC) as rn,
				   COUNT(*) OVER (PARTITION BY contact_id) as msg_count
			FROM messages
		),
		SeqInfo AS (
			SELECT sc.contact_id, 
			       COUNT(sc.sequence_id) as seq_count,
				   GROUP_CONCAT(sc.sequence_id || ':' || s.status) as sequence_ids,
				   MAX(s.status) as last_seq_status
			FROM sequence_contacts sc
			JOIN sequences s ON sc.sequence_id = s.id
			GROUP BY sc.contact_id
		)
		SELECT c.*, i.account_id,
		       COALESCE(m.text, '') as last_message,
			   COALESCE(datetime(m.timestamp), datetime(c.created_at)) as last_time,
			   COALESCE(m.is_incoming, 0) as last_is_incoming,
			   COALESCE(m.msg_count, 0) as msg_count,
			   CASE WHEN si.seq_count > 0 THEN 1 ELSE 0 END as in_sequence,
			   COALESCE(si.last_seq_status, '') as seq_status,
			   COALESCE(si.sequence_ids, '') as sequence_ids
		FROM contacts c
		JOIN integrations i ON c.integration_id = i.id
		LEFT JOIN LatestMessages m ON c.id = m.contact_id AND m.rn = 1
		LEFT JOIN SeqInfo si ON c.id = si.contact_id
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
	return r.UpsertTGContactExt(ctx, r.db, integrationID, externalID, firstName, lastName, username, accessHash)
}

func (r *ContactRepository) UpsertTGContactExt(ctx context.Context, ext sqlx.ExtContext, integrationID int64, externalID, firstName, lastName, username string, accessHash int64) error {
	_, err := ext.ExecContext(ctx, `
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

func (r *ContactRepository) UpsertHHContact(ctx context.Context, integrationID int64, externalID, firstName, lastName, username string) error {
	return r.UpsertHHContactExt(ctx, r.db, integrationID, externalID, firstName, lastName, username)
}

func (r *ContactRepository) UpsertHHContactExt(ctx context.Context, ext sqlx.ExtContext, integrationID int64, externalID, firstName, lastName, username string) error {
	_, err := ext.ExecContext(ctx, `
		INSERT INTO contacts (integration_id, platform, external_id, first_name, last_name, username, access_hash) 
		VALUES (?, 'hh', ?, ?, ?, ?, 0)
		ON CONFLICT(external_id) DO UPDATE SET 
			first_name = excluded.first_name,
			last_name = excluded.last_name,
			username = excluded.username,
			access_hash = 0
	`, integrationID, externalID, firstName, lastName, username)
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

func (r *ContactRepository) UpdateIgnored(ctx context.Context, id interface{}, ignored bool) error {
	_, err := r.db.ExecContext(ctx, "UPDATE contacts SET is_ignored = ? WHERE id = ?", ignored, id)
	return err
}
