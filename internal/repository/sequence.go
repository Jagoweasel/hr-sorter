package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type SequenceWithDetails struct {
	models.Sequence
	Recruiters []models.Contact
	Stages     []models.InterviewStage
	History    []models.InterviewStage
	IsRejected bool
	IsAccepted bool
}

type SequenceRepository struct {
	db *sqlx.DB
}

func NewSequenceRepository(db *sqlx.DB) *SequenceRepository {
	return &SequenceRepository{db: db}
}

func (r *SequenceRepository) GetAll(ctx context.Context, accountID string) ([]models.Sequence, error) {
	var sequences []models.Sequence
	query := "SELECT * FROM sequences"
	var args []interface{}
	if accountID != "" {
		query += " WHERE account_id = ?"
		args = append(args, accountID)
	}
	query += " ORDER BY created_at DESC"
	err := r.db.SelectContext(ctx, &sequences, query, args...)
	return sequences, err
}

func (r *SequenceRepository) GetByID(ctx context.Context, id interface{}) (*models.Sequence, error) {
	var s models.Sequence
	err := r.db.GetContext(ctx, &s, "SELECT * FROM sequences WHERE id = ?", id)
	return &s, err
}

func (r *SequenceRepository) UpdateStatus(ctx context.Context, id interface{}, status string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE sequences SET status = ? WHERE id = ?", status, id)
	return err
}

func (r *SequenceRepository) Delete(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM sequences WHERE id = ?", id)
	return err
}

func (r *SequenceRepository) Create(ctx context.Context, tx *sqlx.Tx, accountID *int64, company, vacancy, status string) (int64, error) {
	res, err := tx.ExecContext(ctx, "INSERT INTO sequences (account_id, company_name, vacancy_name, status) VALUES (?, ?, ?, ?)", accountID, company, vacancy, status)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (r *SequenceRepository) LinkContact(ctx context.Context, tx *sqlx.Tx, sequenceID, contactID interface{}) error {
	query := "INSERT INTO sequence_contacts (sequence_id, contact_id) VALUES (?, ?)"
	if tx != nil {
		_, err := tx.ExecContext(ctx, query, sequenceID, contactID)
		return err
	}
	_, err := r.db.ExecContext(ctx, "INSERT OR IGNORE INTO sequence_contacts (sequence_id, contact_id) VALUES (?, ?)", sequenceID, contactID)
	return err
}

func (r *SequenceRepository) GetRecruiters(ctx context.Context, sequenceID int64) ([]models.Contact, error) {
	var recruiters []models.Contact
	err := r.db.SelectContext(ctx, &recruiters, `
		SELECT c.* FROM contacts c 
		JOIN sequence_contacts sc ON c.id = sc.contact_id 
		WHERE sc.sequence_id = ?`, sequenceID)
	return recruiters, err
}

func (r *SequenceRepository) GetStages(ctx context.Context, sequenceID int64) ([]models.InterviewStage, error) {
	var stages []models.InterviewStage
	err := r.db.SelectContext(ctx, &stages, "SELECT * FROM interview_stages WHERE sequence_id = ? ORDER BY order_index ASC", sequenceID)
	return stages, err
}

func (r *SequenceRepository) GetStageByID(ctx context.Context, id interface{}) (*models.InterviewStage, error) {
	var s models.InterviewStage
	err := r.db.GetContext(ctx, &s, "SELECT * FROM interview_stages WHERE id = ?", id)
	return &s, err
}

func (r *SequenceRepository) UpdateStageStatus(ctx context.Context, id interface{}, completed bool) error {
	_, err := r.db.ExecContext(ctx, "UPDATE interview_stages SET is_completed = ? WHERE id = ?", completed, id)
	return err
}

func (r *SequenceRepository) DeleteStage(ctx context.Context, id interface{}) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM interview_stages WHERE id = ?", id)
	return err
}

func (r *SequenceRepository) CreateStage(ctx context.Context, tx *sqlx.Tx, sequenceID int64, name string, scheduledAt interface{}, completed bool, orderIndex int) error {
	query := "INSERT INTO interview_stages (sequence_id, name, scheduled_at, is_completed, order_index) VALUES (?, ?, ?, ?, ?)"
	var err error
	if tx != nil {
		_, err = tx.ExecContext(ctx, query, sequenceID, name, scheduledAt, completed, orderIndex)
	} else {
		_, err = r.db.ExecContext(ctx, query, sequenceID, name, scheduledAt, completed, orderIndex)
	}
	return err
}

func (r *SequenceRepository) GetLastCompletedStage(ctx context.Context, sequenceID int64) (*models.InterviewStage, error) {
	var stages []models.InterviewStage
	err := r.db.SelectContext(ctx, &stages, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 1 ORDER BY order_index DESC LIMIT 1", sequenceID)
	if err != nil || len(stages) == 0 {
		return nil, err
	}
	return &stages[0], nil
}

func (r *SequenceRepository) GetStageCountByCategory(ctx context.Context, sequenceID int64, label, category string) (int, error) {
	var count int
	err := r.db.GetContext(ctx, &count, "SELECT count(*) FROM interview_stages WHERE sequence_id = ? AND (name LIKE ? OR name LIKE ?)",
		sequenceID, label+"%", category+"%")
	return count, err
}

func (r *SequenceRepository) ShiftStages(ctx context.Context, sequenceID int64, insertAt int) error {
	_, err := r.db.ExecContext(ctx, "UPDATE interview_stages SET order_index = order_index + 1 WHERE sequence_id = ? AND order_index >= ?", sequenceID, insertAt)
	return err
}

func (r *SequenceRepository) GetFirstIncompleteStage(ctx context.Context, sequenceID interface{}) (*models.InterviewStage, error) {
	var s models.InterviewStage
	err := r.db.GetContext(ctx, &s, "SELECT * FROM interview_stages WHERE sequence_id = ? AND is_completed = 0 ORDER BY order_index ASC LIMIT 1", sequenceID)
	return &s, err
}

func (r *SequenceRepository) BeginTx(ctx context.Context) (*sqlx.Tx, error) {
	return r.db.BeginTxx(ctx, nil)
}
