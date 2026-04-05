package hh

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *sqlx.DB {
	db, err := sqlx.Open("sqlite3", ":memory:")
	require.NoError(t, err)

	schema := `
	CREATE TABLE integrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER NOT NULL,
		platform TEXT NOT NULL,
		identifier TEXT,
		access_token TEXT,
		refresh_token TEXT,
		expires_at DATETIME,
		user_agent TEXT,
		status TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(account_id, platform)
	);`
	_, err = db.Exec(schema)
	require.NoError(t, err)

	return db
}

func TestSQLiteSessionStorage_SaveAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewSQLiteSessionStorage(db)
	ctx := context.Background()

	session := &Session{
		AccountID:    1,
		Identifier:   "test@example.com",
		AccessToken:  "access",
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(time.Hour).Truncate(time.Second),
		UserAgent:    "ua",
	}

	err := storage.Save(ctx, session)
	require.NoError(t, err)

	got, err := storage.Get(ctx, 1)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, session.AccountID, got.AccountID)
	assert.Equal(t, session.Identifier, got.Identifier)
	assert.Equal(t, session.AccessToken, got.AccessToken)
	assert.Equal(t, session.RefreshToken, got.RefreshToken)
	assert.True(t, session.ExpiresAt.Equal(got.ExpiresAt))
	assert.Equal(t, session.UserAgent, got.UserAgent)
}

func TestSQLiteSessionStorage_Delete(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	storage := NewSQLiteSessionStorage(db)
	ctx := context.Background()

	session := &Session{AccountID: 1, Identifier: "test"}
	_ = storage.Save(ctx, session)

	err := storage.Delete(ctx, 1)
	require.NoError(t, err)

	got, err := storage.Get(ctx, 1)
	require.NoError(t, err)
	assert.Nil(t, got)
}
