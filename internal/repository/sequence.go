package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type SequenceWithDetails struct {
	models.Sequence
	AccountName string
	AccountSlug string
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
		AccountSlug string `db:"account_slug"`
	}
	query := `
		SELECT s.*, 
		       COALESCE(a.name, 'Unknown') as account_name,
		       COALESCE(a.slug, '') as account_slug
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

	if len(sequences) == 0 {
		return nil, nil
	}

	sequenceIDs := make([]int64, len(sequences))
	for i, s := range sequences {
		sequenceIDs[i] = s.ID
	}

	recruitersMap, err := r.GetRecruitersBatch(ctx, sequenceIDs)
	if err != nil {
		return nil, err
	}
	stagesMap, err := r.GetStagesBatch(ctx, sequenceIDs)
	if err != nil {
		return nil, err
	}

	var detailed []SequenceWithDetails
	for _, s := range sequences {
		recruiters := recruitersMap[s.ID]
		if recruiters == nil {
			recruiters = []models.Contact{}
		}
		stages := stagesMap[s.ID]
		if stages == nil {
			stages = []models.InterviewStage{}
		}

		var history []models.InterviewStage
		for _, st := range stages {
			if st.IsCompleted {
				history = append(history, st)
			}
		}

		detailed = append(detailed, SequenceWithDetails{
			Sequence:    s.Sequence,
			AccountName: s.AccountName,
			AccountSlug: s.AccountSlug,
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

func (r *SequenceRepository) GetRecruitersBatch(ctx context.Context, sequenceIDs []int64) (map[int64][]models.Contact, error) {
	if len(sequenceIDs) == 0 {
		return make(map[int64][]models.Contact), nil
	}
	query, args, err := sqlx.In(`
		SELECT sc.sequence_id, c.* FROM contacts c 
		JOIN sequence_contacts sc ON c.id = sc.contact_id 
		WHERE sc.sequence_id IN (?)`, sequenceIDs)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)

	var results []struct {
		SequenceID int64 `db:"sequence_id"`
		models.Contact
	}
	err = r.db.SelectContext(ctx, &results, query, args...)
	if err != nil {
		return nil, err
	}

	res := make(map[int64][]models.Contact)
	for _, r := range results {
		res[r.SequenceID] = append(res[r.SequenceID], r.Contact)
	}
	return res, nil
}

func (r *SequenceRepository) GetStagesBatch(ctx context.Context, sequenceIDs []int64) (map[int64][]models.InterviewStage, error) {
	if len(sequenceIDs) == 0 {
		return make(map[int64][]models.InterviewStage), nil
	}
	query, args, err := sqlx.In("SELECT * FROM interview_stages WHERE sequence_id IN (?) ORDER BY order_index ASC", sequenceIDs)
	if err != nil {
		return nil, err
	}
	query = r.db.Rebind(query)

	var stages []models.InterviewStage
	err = r.db.SelectContext(ctx, &stages, query, args...)
	if err != nil {
		return nil, err
	}

	res := make(map[int64][]models.InterviewStage)
	for _, s := range stages {
		res[s.SequenceID] = append(res[s.SequenceID], s)
	}
	return res, nil
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

func (r *SequenceRepository) GetTotalApplications(ctx context.Context, accountID string) (int, error) {
	// Fallback count: unique contacts from HH plus negotiations_stats
	query := `
		SELECT (
			SELECT COALESCE(SUM(applications_count), 0) FROM negotiations_stats ns
			JOIN integrations i ON ns.integration_id = i.id
			WHERE (i.account_id = ? OR ? = '')
		) + (
			SELECT COUNT(*) FROM contacts c
			JOIN integrations i ON c.integration_id = i.id
			WHERE c.platform = 'hh' 
			AND (i.account_id = ? OR ? = '')
			AND c.external_id NOT IN (SELECT 'hh_neg_' || CAST(integration_id AS TEXT) FROM negotiations_stats) -- avoid double counting if possible, though logic varies
		)`

	// Actually, a simpler and more reliable fallback:
	// Use the MAX of negotiations_stats OR the actual count of HH contacts in our DB
	query = `
		SELECT MAX(apps.val) FROM (
			SELECT COALESCE(SUM(ns.applications_count), 0) as val
			FROM negotiations_stats ns
			JOIN integrations i ON ns.integration_id = i.id
			WHERE (i.account_id = ? OR ? = '')
			UNION ALL
			SELECT COUNT(*) as val
			FROM contacts c
			JOIN integrations i ON c.integration_id = i.id
			WHERE c.platform = 'hh' AND (i.account_id = ? OR ? = '')
		) as apps`

	var count int
	err := r.db.GetContext(ctx, &count, query, accountID, accountID, accountID, accountID)
	return count, err
}
