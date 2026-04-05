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
	_, err = repo.GetAccount(ctx, "1")
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
	cachedAcc, err := repo.GetAccount(ctx, "1")
	if err != nil {
		t.Fatalf("failed to get account: %v", err)
	}
	if cachedAcc.(*models.Account).Name != "Test Account" {
		t.Errorf("expected cached name 'Test Account', got %s", cachedAcc.(*models.Account).Name)
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
