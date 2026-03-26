package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type SequenceWithDetails struct {
	models.Sequence
	AccountName string
	Recruiters  []models.Contact
	Stages      []models.InterviewStage
	History     []models.InterviewStage
	IsRejected  bool
	IsAccepted  bool
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

func (r *SequenceRepository) UpdateDetails(ctx context.Context, id interface{}, company, vacancy, link, reason, comment string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE sequences SET company_name = ?, vacancy_name = ?, vacancy_link = ?, rejection_reason = ?, summary_comment = ? WHERE id = ?", company, vacancy, link, reason, comment, id)
	return err
}

func (r *SequenceRepository) GetAllFullDetails(ctx context.Context, accountID string) ([]SequenceWithDetails, error) {
	var sequences []struct {
		models.Sequence
		AccountName string `db:"account_name"`
	}
	query := `
		SELECT s.*, COALESCE(a.name, 'Unknown') as account_name 
		FROM sequences s 
		LEFT JOIN accounts a ON s.account_id = a.id`
	var args []interface{}
	if accountID != "" {
		query += " WHERE s.account_id = ?"
		args = append(args, accountID)
	}
	query += " ORDER BY s.created_at DESC"
	err := r.db.SelectContext(ctx, &sequences, query, args...)
	if err != nil {
		return nil, err
	}

	var detailed []SequenceWithDetails
	for _, s := range sequences {
		recruiters, _ := r.GetRecruiters(ctx, s.ID)
		stages, _ := r.GetStages(ctx, s.ID)

		var history []models.InterviewStage
		for _, st := range stages {
			if st.IsCompleted {
				history = append(history, st)
			}
		}

		detailed = append(detailed, SequenceWithDetails{
			Sequence:    s.Sequence,
			AccountName: s.AccountName,
			Recruiters:  recruiters,
			Stages:      stages,
			History:     history,
			IsRejected:  s.Status == "rejected",
			IsAccepted:  s.Status == "accepted",
		})
	}

	return detailed, nil
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

type StatusCount struct {
	Status string `db:"status"`
	Count  int    `db:"count"`
}

type VacancyStats struct {
	VacancyName string `db:"vacancy_name"`
	Status      string `db:"status"`
	Count       int    `db:"count"`
}

type CompanyStats struct {
	CompanyName string `db:"company_name"`
	Status      string `db:"status"`
	Count       int    `db:"count"`
}

type PlatformStats struct {
	Platform string `db:"platform"`
	Count    int    `db:"count"`
}

func (r *SequenceRepository) GetStatusCounts(ctx context.Context, accountID string) ([]StatusCount, error) {
	var counts []StatusCount
	query := "SELECT status, count(*) as count FROM sequences"
	var args []interface{}
	if accountID != "" {
		query += " WHERE account_id = ?"
		args = append(args, accountID)
	}
	query += " GROUP BY status"
	err := r.db.SelectContext(ctx, &counts, query, args...)
	return counts, err
}

func (r *SequenceRepository) GetVacancyStats(ctx context.Context, accountID string) ([]VacancyStats, error) {
	var stats []VacancyStats
	query := "SELECT vacancy_name, status, count(*) as count FROM sequences"
	var args []interface{}
	if accountID != "" {
		query += " WHERE account_id = ?"
		args = append(args, accountID)
	}
	query += " GROUP BY vacancy_name, status"
	err := r.db.SelectContext(ctx, &stats, query, args...)
	return stats, err
}

func (r *SequenceRepository) GetCompanyStats(ctx context.Context, accountID string) ([]CompanyStats, error) {
	var stats []CompanyStats
	query := "SELECT company_name, status, count(*) as count FROM sequences"
	var args []interface{}
	if accountID != "" {
		query += " WHERE account_id = ?"
		args = append(args, accountID)
	}
	query += " GROUP BY company_name, status"
	err := r.db.SelectContext(ctx, &stats, query, args...)
	return stats, err
}

func (r *SequenceRepository) GetPlatformStats(ctx context.Context, accountID string) ([]PlatformStats, error) {
	var stats []PlatformStats
	query := `
		SELECT i.platform, count(DISTINCT s.id) as count 
		FROM sequences s
		JOIN sequence_contacts sc ON s.id = sc.sequence_id
		JOIN contacts c ON sc.contact_id = c.id
		JOIN integrations i ON c.integration_id = i.id`
	var args []interface{}
	if accountID != "" {
		query += " WHERE s.account_id = ?"
		args = append(args, accountID)
	}
	query += " GROUP BY i.platform"
	err := r.db.SelectContext(ctx, &stats, query, args...)
	return stats, err
}
