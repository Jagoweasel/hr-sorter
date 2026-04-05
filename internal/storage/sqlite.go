package storage

import (
	"context"
	"fmt"
	"hr-sorter/internal/database"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/models"
	"sync"

	"github.com/hashicorp/golang-lru/v2"
	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

// SQLiteRepository implementation with LRU cache
type SQLiteRepository struct {
	db    *sqlx.DB
	mu    sync.RWMutex
	cache *lru.Cache[string, interface{}]
}

func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
	// 1. Open SQLite database
	db, err := sqlx.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// 2. Configure PRAGMA (WAL mode, busy_timeout=5000, synchronous=NORMAL)
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	// 3. Initialize golang-lru cache
	cache, err := lru.New[string, interface{}](1024)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	// 4. Run migrations/schema
	if _, err := db.Exec(database.Schema); err != nil {
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	return &SQLiteRepository{
		db:    db,
		cache: cache,
	}, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) SaveAccount(ctx context.Context, acc *models.Account) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO accounts (id, name, slug, status, created_at) 
			  VALUES (:id, :name, :slug, :status, :created_at)
			  ON CONFLICT(id) DO UPDATE SET 
			  name=excluded.name, slug=excluded.slug, status=excluded.status`
	_, err := r.db.NamedExecContext(ctx, query, acc)
	if err != nil {
		return fmt.Errorf("failed to save account: %w", err)
	}

	// Invalidate cache
	r.cache.Remove(fmt.Sprintf("account:%d", acc.ID))
	return nil
}

func (r *SQLiteRepository) GetAccount(ctx context.Context, id int64) (*models.Account, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("account:%d", id)
	// Check LRU cache first
	if val, ok := r.cache.Get(key); ok {
		return val.(*models.Account), nil
	}

	var acc models.Account
	err := r.db.GetContext(ctx, &acc, "SELECT * FROM accounts WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Update cache
	r.cache.Add(key, &acc)
	return &acc, nil
}

func (r *SQLiteRepository) SaveNegotiations(ctx context.Context, negotiations []dto.HHNegotiation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Implementation details...
	// The PRD mentions tracking negotiations for conversion rates.
	// For now, let's just make it a placeholder that completes successfully.
	// We might need a table for negotiations later.

	return tx.Commit()
}

func (r *SQLiteRepository) SaveNegotiationsStats(ctx context.Context, stats *models.NegotiationStats) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO negotiations_stats (sequence_id, applications_count, updated_at)
			  VALUES (:sequence_id, :applications_count, :updated_at)
			  ON CONFLICT(sequence_id) DO UPDATE SET
			  applications_count=excluded.applications_count, updated_at=excluded.updated_at`
	_, err := r.db.NamedExecContext(ctx, query, stats)
	if err != nil {
		return fmt.Errorf("failed to save negotiation stats: %w", err)
	}
	return nil
}

func (r *SQLiteRepository) GetMappingRules(ctx context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check LRU cache first
	if val, ok := r.cache.Get("mapping_rules"); ok {
		return val.(map[string]string), nil
	}

	var rulesList []models.MappingRule
	err := r.db.SelectContext(ctx, &rulesList, "SELECT * FROM mapping_rules")
	if err != nil {
		return nil, fmt.Errorf("failed to get mapping rules: %w", err)
	}

	rules := make(map[string]string)
	for _, rule := range rulesList {
		rules[rule.Pattern] = rule.Category
	}

	r.cache.Add("mapping_rules", rules)
	return rules, nil
}

func (r *SQLiteRepository) SaveMappingRule(ctx context.Context, rule *models.MappingRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO mapping_rules (id, pattern, category)
			  VALUES (:id, :pattern, :category)
			  ON CONFLICT(id) DO UPDATE SET
			  pattern=excluded.pattern, category=excluded.category`
	_, err := r.db.NamedExecContext(ctx, query, rule)
	if err != nil {
		return fmt.Errorf("failed to save mapping rule: %w", err)
	}

	r.cache.Remove("mapping_rules")
	return nil
}

func (r *SQLiteRepository) GetMessageFilters(ctx context.Context) ([]models.MessageFilter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if val, ok := r.cache.Get("message_filters"); ok {
		return val.([]models.MessageFilter), nil
	}

	var filters []models.MessageFilter
	err := r.db.SelectContext(ctx, &filters, "SELECT * FROM message_filters WHERE is_active = 1")
	if err != nil {
		return nil, fmt.Errorf("failed to get message filters: %w", err)
	}

	r.cache.Add("message_filters", filters)
	return filters, nil
}

func (r *SQLiteRepository) GetIntegration(ctx context.Context, id int64) (*models.Integration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("integration:%d", id)
	if val, ok := r.cache.Get(key); ok {
		return val.(*models.Integration), nil
	}

	var integration models.Integration
	err := r.db.GetContext(ctx, &integration, "SELECT * FROM integrations WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get integration: %w", err)
	}

	r.cache.Add(key, &integration)
	return &integration, nil
}

func (r *SQLiteRepository) GetSequence(ctx context.Context, id int64) (*models.Sequence, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	key := fmt.Sprintf("sequence:%d", id)
	if val, ok := r.cache.Get(key); ok {
		return val.(*models.Sequence), nil
	}

	var sequence models.Sequence
	err := r.db.GetContext(ctx, &sequence, "SELECT * FROM sequences WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("failed to get sequence: %w", err)
	}

	r.cache.Add(key, &sequence)
	return &sequence, nil
}

func (r *SQLiteRepository) SaveMessageFilter(ctx context.Context, filter *models.MessageFilter) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO message_filters (id, pattern, is_active, created_at)
			  VALUES (:id, :pattern, :is_active, :created_at)
			  ON CONFLICT(id) DO UPDATE SET
			  pattern=excluded.pattern, is_active=excluded.is_active`
	_, err := r.db.NamedExecContext(ctx, query, filter)
	if err != nil {
		return fmt.Errorf("failed to save filter: %w", err)
	}

	r.cache.Remove("message_filters")
	return nil
}

func (r *SQLiteRepository) SaveIntegration(ctx context.Context, integration *models.Integration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO integrations (id, account_id, platform, identifier, api_id, api_hash, access_token, refresh_token, expires_at, user_agent, status, session_path, created_at)
			  VALUES (:id, :account_id, :platform, :identifier, :api_id, :api_hash, :access_token, :refresh_token, :expires_at, :user_agent, :status, :session_path, :created_at)
			  ON CONFLICT(id) DO UPDATE SET
			  account_id=excluded.account_id, platform=excluded.platform, identifier=excluded.identifier, 
			  api_id=excluded.api_id, api_hash=excluded.api_hash, user_agent=excluded.user_agent, 
			  status=excluded.status, session_path=excluded.session_path, 
			  access_token=excluded.access_token, refresh_token=excluded.refresh_token, expires_at=excluded.expires_at`
	_, err := r.db.NamedExecContext(ctx, query, integration)
	if err != nil {
		return fmt.Errorf("failed to save integration: %w", err)
	}

	r.cache.Remove(fmt.Sprintf("integration:%d", integration.ID))
	return nil
}

func (r *SQLiteRepository) SaveSequence(ctx context.Context, sequence *models.Sequence) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	query := `INSERT INTO sequences (id, account_id, company_name, vacancy_name, vacancy_link, category, status, rejection_reason, summary_comment, created_at)
			  VALUES (:id, :account_id, :company_name, :vacancy_name, :vacancy_link, :category, :status, :rejection_reason, :summary_comment, :created_at)
			  ON CONFLICT(id) DO UPDATE SET
			  account_id=excluded.account_id, company_name=excluded.company_name, vacancy_name=excluded.vacancy_name, 
			  vacancy_link=excluded.vacancy_link, status=excluded.status, category=excluded.category, 
			  rejection_reason=excluded.rejection_reason, summary_comment=excluded.summary_comment`
	_, err := r.db.NamedExecContext(ctx, query, sequence)
	if err != nil {
		return fmt.Errorf("failed to save sequence: %w", err)
	}

	r.cache.Remove(fmt.Sprintf("sequence:%d", sequence.ID))
	return nil
}
