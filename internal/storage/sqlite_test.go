package storage

import (
	"context"
	"hr-sorter/internal/models"
	"os"
	"testing"
	"time"
)

func TestNewSQLiteRepository(t *testing.T) {
	dbPath := "test.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	if repo.db == nil {
		t.Fatal("db is nil")
	}

	// Verify WAL mode
	var journalMode string
	err = repo.db.Get(&journalMode, "PRAGMA journal_mode")
	if err != nil {
		t.Fatalf("failed to get journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("expected journal mode wal, got %s", journalMode)
	}

	// Verify synchronous NORMAL
	var synchronous int
	err = repo.db.Get(&synchronous, "PRAGMA synchronous")
	if err != nil {
		t.Fatalf("failed to get synchronous: %v", err)
	}
	if synchronous != 1 { // 1 = NORMAL
		t.Errorf("expected synchronous 1, got %d", synchronous)
	}
}

func TestSQLiteRepository_Indexes(t *testing.T) {
	dbPath := "test_indexes.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	// Verify idx_messages_integration_timestamp
	var count int
	err = repo.db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_messages_integration_timestamp'")
	if err != nil {
		t.Fatalf("failed to check for index: %v", err)
	}
	if count == 0 {
		t.Error("index idx_messages_integration_timestamp not found")
	}

	// Verify idx_sequences_account_status
	err = repo.db.Get(&count, "SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_sequences_account_status'")
	if err != nil {
		t.Fatalf("failed to check for index: %v", err)
	}
	if count == 0 {
		t.Error("index idx_sequences_account_status not found")
	}
}

func TestSQLiteRepository_Caching(t *testing.T) {
	dbPath := "test_cache.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Insert test account
	acc := &models.Account{
		ID:        1,
		Name:      "Test Account",
		Status:    "active",
		CreatedAt: time.Now(),
	}

	err = repo.SaveAccount(ctx, acc)
	if err != nil {
		t.Fatalf("failed to save account: %v", err)
	}

	// 1. Get account (should cache)
	_, err = repo.GetAccount(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}

	if _, ok := repo.cache.Get("account:1"); !ok {
		t.Error("account 1 not found in cache")
	}

	// 2. Modify account in DB directly (bypass repo)
	_, err = repo.db.Exec("UPDATE accounts SET name = 'Updated' WHERE id = 1")
	if err != nil {
		t.Fatalf("failed to update account in db: %v", err)
	}

	// 3. Get account again (should still be from cache)
	cachedAcc, err := repo.GetAccount(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}
	if cachedAcc.Name != "Test Account" {
		t.Errorf("expected cached name 'Test Account', got %s", cachedAcc.Name)
	}

	// 4. Update through repo (should invalidate cache)
	acc.Name = "New Name"
	err = repo.SaveAccount(ctx, acc)
	if err != nil {
		t.Fatalf("failed to save account through repo: %v", err)
	}

	if _, ok := repo.cache.Get("account:1"); ok {
		t.Error("account 1 still in cache after save")
	}
}

func TestSQLiteRepository_Concurrency(t *testing.T) {
	dbPath := "test_concurrency.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// Initial data
	acc := &models.Account{
		ID:        1,
		Name:      "Concurrent Account",
		Status:    "active",
		CreatedAt: time.Now(),
	}
	_ = repo.SaveAccount(ctx, acc)

	const (
		numGoroutines = 50
		numOpsPerG    = 100
	)

	done := make(chan bool)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			for j := 0; j < numOpsPerG; j++ {
				if j%10 == 0 {
					// Save account with a new struct to avoid data race in the test itself
					newAcc := &models.Account{
						ID:        1,
						Name:      "Updated",
						Status:    "active",
						CreatedAt: time.Now(),
					}
					_ = repo.SaveAccount(ctx, newAcc)
				} else {
					// Get account
					_, _ = repo.GetAccount(ctx, 1)
				}
			}
			done <- true
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

func TestSQLiteRepository_SaveIntegration_Upsert(t *testing.T) {
	dbPath := "test_integration_upsert.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 0. Create account
	acc := &models.Account{
		ID:     1,
		Name:   "Main Account",
		Status: "active",
	}
	_ = repo.SaveAccount(ctx, acc)

	// 1. Initial save
	integration := &models.Integration{
		ID:          1,
		AccountID:   1,
		Platform:    "tg",
		Identifier:  "123456789",
		APIID:       123,
		APIHash:     "hash123",
		Status:      "active",
		SessionPath: "path/to/session",
		UserAgent:   ptr("Mozilla/5.0"),
		CreatedAt:   time.Now().Truncate(time.Second),
	}

	err = repo.SaveIntegration(ctx, integration)
	if err != nil {
		t.Fatalf("failed to save integration: %v", err)
	}

	// 2. Update with new values
	integration.APIID = 456
	integration.APIHash = "hash456"
	integration.SessionPath = "new/path"
	integration.UserAgent = ptr("New User Agent")
	integration.Status = "inactive"

	err = repo.SaveIntegration(ctx, integration)
	if err != nil {
		t.Fatalf("failed to update integration: %v", err)
	}

	// 3. Verify updates
	saved, err := repo.GetIntegration(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get integration: %v", err)
	}

	if saved.APIID != 456 {
		t.Errorf("expected APIID 456, got %d", saved.APIID)
	}
	if saved.APIHash != "hash456" {
		t.Errorf("expected APIHash 'hash456', got %s", saved.APIHash)
	}
	if saved.SessionPath != "new/path" {
		t.Errorf("expected SessionPath 'new/path', got %s", saved.SessionPath)
	}
	if saved.UserAgent == nil || *saved.UserAgent != "New User Agent" {
		var val string
		if saved.UserAgent != nil {
			val = *saved.UserAgent
		}
		t.Errorf("expected UserAgent 'New User Agent', got %s", val)
	}
	if saved.Status != "inactive" {
		t.Errorf("expected Status 'inactive', got %s", saved.Status)
	}
}

func TestSQLiteRepository_SaveSequence_Upsert(t *testing.T) {
	dbPath := "test_sequence_upsert.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 1. Initial save
	sequence := &models.Sequence{
		ID:          1,
		CompanyName: "Company A",
		VacancyName: "Developer",
		VacancyLink: ptr("http://link.a"),
		Status:      "initial",
		CreatedAt:   time.Now().Truncate(time.Second),
	}

	err = repo.SaveSequence(ctx, sequence)
	if err != nil {
		t.Fatalf("failed to save sequence: %v", err)
	}

	// 2. Update with new values
	sequence.CompanyName = "Company B"
	sequence.VacancyName = "Senior Developer"
	sequence.VacancyLink = ptr("http://link.b")
	sequence.Status = "screening"

	err = repo.SaveSequence(ctx, sequence)
	if err != nil {
		t.Fatalf("failed to update sequence: %v", err)
	}

	// 3. Verify updates
	saved, err := repo.GetSequence(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get sequence: %v", err)
	}

	if saved.CompanyName != "Company B" {
		t.Errorf("expected CompanyName 'Company B', got %s", saved.CompanyName)
	}
	if saved.VacancyName != "Senior Developer" {
		t.Errorf("expected VacancyName 'Senior Developer', got %s", saved.VacancyName)
	}
	if saved.VacancyLink == nil || *saved.VacancyLink != "http://link.b" {
		var val string
		if saved.VacancyLink != nil {
			val = *saved.VacancyLink
		}
		t.Errorf("expected VacancyLink 'http://link.b', got %s", val)
	}
	if saved.Status != "screening" {
		t.Errorf("expected Status 'screening', got %s", saved.Status)
	}
}

func TestSQLiteRepository_SaveMappingRule(t *testing.T) {
	dbPath := "test_mapping_rule.db"
	defer os.Remove(dbPath)

	repo, err := NewSQLiteRepository(dbPath)
	if err != nil {
		t.Fatalf("failed to create repository: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()

	// 1. Initial save
	rule := &models.MappingRule{
		ID:       1,
		Pattern:  "python",
		Category: "Backend",
	}

	err = repo.SaveMappingRule(ctx, rule)
	if err != nil {
		t.Fatalf("failed to save mapping rule: %v", err)
	}

	// 2. Cache check
	rules, err := repo.GetMappingRules(ctx)
	if err != nil {
		t.Fatalf("failed to get mapping rules: %v", err)
	}
	if rules["python"] != "Backend" {
		t.Errorf("expected category 'Backend', got %s", rules["python"])
	}

	// 3. Update rule
	rule.Category = "Data Science"
	err = repo.SaveMappingRule(ctx, rule)
	if err != nil {
		t.Fatalf("failed to update mapping rule: %v", err)
	}

	// 4. Verify update (should invalidate cache and get from DB)
	rules, err = repo.GetMappingRules(ctx)
	if err != nil {
		t.Fatalf("failed to get mapping rules: %v", err)
	}
	if rules["python"] != "Data Science" {
		t.Errorf("expected category 'Data Science', got %s", rules["python"])
	}
}

func ptr[T any](v T) *T {
	return &v
}
