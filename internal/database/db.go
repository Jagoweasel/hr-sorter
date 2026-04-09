package database

import (
	_ "embed"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var Schema string

var DB *sqlx.DB

func InitDB(path string) {
	log.Printf("[DB] Initializing SQLite database at %s...", path)
	var err error
	dsn := path
	if !strings.Contains(path, "?") {
		dsn = path + "?_pragma=foreign_keys=1&_journal_mode=WAL&_busy_timeout=30000&_txlock=immediate"
	}
	log.Printf("[DB] Connecting with DSN: %s", dsn)
	DB, err = sqlx.Connect("sqlite", dsn)
	if err != nil {
		log.Fatalf("[DB] Failed to connect to database: %v", err)
	}
	// SQLite only supports one writer at a time, and concurrent readers can conflict with writers.
	// In WAL mode, multiple readers can exist alongside one writer.
	// busy_timeout (set in DSN) handles write serialization.
	DB.SetMaxOpenConns(10)
	DB.SetMaxIdleConns(5)
	log.Println("[DB] Connection pool configured (MaxOpen: 10, MaxIdle: 5).")
	log.Println("[DB] Connection established.")

	log.Println("[DB] Verifying schema...")

	// Check if we need to migrate from old accounts table
	var hasIntegrations bool
	err = DB.Get(&hasIntegrations, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='integrations'")

	var hasAccounts bool
	_ = DB.Get(&hasAccounts, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='accounts'")

	if err == nil && !hasIntegrations && hasAccounts {
		log.Println("[DB] Old schema detected. Starting migration...")

		// 1. Rename old tables
		DB.MustExec("ALTER TABLE accounts RENAME TO accounts_old")
		DB.MustExec("ALTER TABLE contacts RENAME TO contacts_old")
		DB.MustExec("ALTER TABLE messages RENAME TO messages_old")

		// 2. Create new schema
		DB.MustExec(Schema)

		// 3. Create a default account
		var accountID int64
		res := DB.MustExec("INSERT INTO accounts (name, status) VALUES ('Default Account', 'active')")
		accountID, _ = res.LastInsertId()

		// 4. Migrate integrations
		DB.MustExec(`
			INSERT INTO integrations (account_id, platform, identifier, status, session_path, created_at)
			SELECT ?, 'tg', phone_number, status, session_path, created_at FROM accounts_old
		`, accountID)

		// 5. Migrate contacts
		// We need to link contacts to their respective integration.
		// Since we only had one account type before, we just link to the first integration of 'tg' type.
		var integrationID int64
		DB.Get(&integrationID, "SELECT id FROM integrations WHERE platform = 'tg' LIMIT 1")

		DB.MustExec(`
			INSERT INTO contacts (integration_id, platform, external_id, first_name, last_name, username, created_at)
			SELECT ?, 'tg', CAST(tg_user_id AS TEXT), first_name, last_name, username, created_at FROM contacts_old
		`, integrationID)

		// 6. Migrate messages
		// We map contacts_old ids to new contacts ids.
		DB.MustExec(`
			INSERT INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp)
			SELECT ?, n.id, CAST(o.tg_message_id AS TEXT), o.text, o.is_incoming, o.timestamp
			FROM messages_old o
			JOIN contacts n ON CAST(o.contact_id AS TEXT) = n.external_id
		`, integrationID)

		// 7. Update sequences
		DB.MustExec("UPDATE sequences SET account_id = ?", accountID)

		log.Println("[DB] Migration finished.")
	} else {
		DB.MustExec(Schema)
		log.Println("[DB] Schema verified.")
	}

	// Migration: Add api_id and api_hash to integrations if they don't exist
	var hasAPIID bool
	err = DB.Get(&hasAPIID, "SELECT COUNT(*) FROM pragma_table_info('integrations') WHERE name='api_id'")
	if err == nil && !hasAPIID {
		log.Println("[DB] Migrating integrations: adding api_id and api_hash columns...")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN api_id INTEGER")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN api_hash TEXT")

		// Populate existing integrations with .env values as fallback
		apiID, _ := strconv.Atoi(os.Getenv("API_ID"))
		apiHash := os.Getenv("API_HASH")
		if apiID != 0 && apiHash != "" {
			DB.MustExec("UPDATE integrations SET api_id = ?, api_hash = ? WHERE platform = 'tg'", apiID, apiHash)
		}
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add access_hash to contacts if it doesn't exist
	var hasAccessHash bool
	err = DB.Get(&hasAccessHash, "SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name='access_hash'")
	if err == nil && !hasAccessHash {
		log.Println("[DB] Migrating contacts: adding access_hash column...")
		DB.MustExec("ALTER TABLE contacts ADD COLUMN access_hash INTEGER DEFAULT 0")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add HH columns to integrations if they don't exist
	var hasHHColumns bool
	err = DB.Get(&hasHHColumns, "SELECT COUNT(*) FROM pragma_table_info('integrations') WHERE name='access_token'")
	if err == nil && !hasHHColumns {
		log.Println("[DB] Migrating integrations: adding HH session columns...")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN access_token TEXT")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN refresh_token TEXT")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN expires_at DATETIME")
		DB.MustExec("ALTER TABLE integrations ADD COLUMN user_agent TEXT")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add is_ignored to contacts if it doesn't exist
	var hasIsIgnored bool
	err = DB.Get(&hasIsIgnored, "SELECT COUNT(*) FROM pragma_table_info('contacts') WHERE name='is_ignored'")
	if err == nil && !hasIsIgnored {
		log.Println("[DB] Migrating contacts: adding is_ignored column...")
		DB.MustExec("ALTER TABLE contacts ADD COLUMN is_ignored BOOLEAN DEFAULT 0")
		log.Println("[DB] Migration finished.")
	}

	// Ensure NO nulls in access_hash if it was already there without default
	DB.MustExec("UPDATE contacts SET access_hash = 0 WHERE access_hash IS NULL")

	// Migration: Add report columns to sequences if they don't exist
	var hasReportColumns bool
	err = DB.Get(&hasReportColumns, "SELECT COUNT(*) FROM pragma_table_info('sequences') WHERE name='vacancy_link'")
	if err == nil && !hasReportColumns {
		log.Println("[DB] Migrating sequences: adding report columns...")
		DB.MustExec("ALTER TABLE sequences ADD COLUMN vacancy_link TEXT")
		DB.MustExec("ALTER TABLE sequences ADD COLUMN rejection_reason TEXT")
		DB.MustExec("ALTER TABLE sequences ADD COLUMN summary_comment TEXT")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add slug to accounts if it doesn't exist
	var hasSlug bool
	err = DB.Get(&hasSlug, "SELECT COUNT(*) FROM pragma_table_info('accounts') WHERE name='slug'")
	if err == nil && !hasSlug {
		log.Println("[DB] Migrating accounts: adding slug column...")
		DB.MustExec("ALTER TABLE accounts ADD COLUMN slug TEXT")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add category to sequences if it doesn't exist
	var hasCategory bool
	err = DB.Get(&hasCategory, "SELECT COUNT(*) FROM pragma_table_info('sequences') WHERE name='category'")
	if err == nil && !hasCategory {
		log.Println("[DB] Migrating sequences: adding category column...")
		DB.MustExec("ALTER TABLE sequences ADD COLUMN category TEXT")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Create mapping_rules and negotiations_stats tables if they don't exist
	DB.MustExec(`
		CREATE TABLE IF NOT EXISTS mapping_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pattern TEXT NOT NULL,
			category TEXT NOT NULL
		);
	`)
	DB.MustExec(`
		CREATE TABLE IF NOT EXISTS negotiations_stats (
			integration_id INTEGER PRIMARY KEY,
			applications_count INTEGER DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
		);
	`)

	// Migration: Add unique index to message_filters pattern
	_, _ = DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_message_filters_pattern ON message_filters(pattern)")

	SeedFilters()
}

func CloseDB() {
	if DB != nil {
		log.Println("[DB] Closing database connection...")
		if err := DB.Close(); err != nil {
			log.Printf("[DB] Error closing database: %v", err)
		} else {
			log.Println("[DB] Database connection closed.")
		}
	}
}

func SeedFilters() {
	path := "filters.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[DB] Error reading filters.json: %v", err)
		return
	}

	var patterns []string
	if err := json.Unmarshal(data, &patterns); err != nil {
		log.Printf("[DB] Error unmarshaling filters.json: %v", err)
		return
	}

	log.Printf("[DB] Seeding %d filters...", len(patterns))
	for _, p := range patterns {
		_, _ = DB.Exec("INSERT OR IGNORE INTO message_filters (pattern, is_active) VALUES (?, 1)", p)
	}
}
