package repository

import (
	"context"
	"hr-sorter/internal/models"

	"github.com/jmoiron/sqlx"
)

type MappingRepository struct {
	db *sqlx.DB
}

func NewMappingRepository(db *sqlx.DB) *MappingRepository {
	return &MappingRepository{db: db}
}

func (r *MappingRepository) GetAll(ctx context.Context) ([]models.MappingRule, error) {
	var rules []models.MappingRule
	err := r.db.SelectContext(ctx, &rules, "SELECT * FROM mapping_rules")
	return rules, err
}

func (r *MappingRepository) Save(ctx context.Context, rule *models.MappingRule) error {
	if rule.ID == 0 {
		_, err := r.db.NamedExecContext(ctx, "INSERT INTO mapping_rules (pattern, category) VALUES (:pattern, :category)", rule)
		return err
	}
	_, err := r.db.NamedExecContext(ctx, "UPDATE mapping_rules SET pattern = :pattern, category = :category WHERE id = :id", rule)
	return err
}

func (r *MappingRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM mapping_rules WHERE id = ?", id)
	return err
}
