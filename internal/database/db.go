package database

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var DB *sqlx.DB

func InitDB(path string) {
	log.Printf("[DB] Initializing SQLite database at %s...", path)
	var err error
	dsn := path
	if !strings.Contains(path, "?") {
		dsn = path + "?_pragma=foreign_keys=1&_journal_mode=WAL&_busy_timeout=5000"
	}
	log.Printf("[DB] Connecting with DSN: %s", dsn)
	DB, err = sqlx.Connect("sqlite", dsn)
	if err != nil {
		log.Fatalf("[DB] Failed to connect to database: %v", err)
	}
	log.Println("[DB] Connection established.")

	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS integrations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER,
		platform TEXT NOT NULL,
		identifier TEXT NOT NULL,
		api_id INTEGER,
		api_hash TEXT,
		status TEXT NOT NULL DEFAULT 'pending_auth',
		session_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE,
		UNIQUE(platform, identifier)
	);

	CREATE TABLE IF NOT EXISTS contacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		integration_id INTEGER,
		platform TEXT NOT NULL DEFAULT 'tg',
		external_id TEXT UNIQUE NOT NULL,
		first_name TEXT,
		last_name TEXT,
		username TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		integration_id INTEGER,
		contact_id INTEGER,
		external_id TEXT,
		text TEXT,
		is_incoming BOOLEAN,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(integration_id, contact_id, external_id),
		FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE,
		FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sequences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER,
		company_name TEXT NOT NULL,
		vacancy_name TEXT NOT NULL,
		status TEXT DEFAULT 'initial',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (account_id) REFERENCES accounts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS sequence_contacts (
		sequence_id INTEGER,
		contact_id INTEGER,
		PRIMARY KEY (sequence_id, contact_id),
		FOREIGN KEY (sequence_id) REFERENCES sequences(id) ON DELETE CASCADE,
		FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS interview_stages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		sequence_id INTEGER,
		name TEXT NOT NULL,
		stage_type TEXT,
		scheduled_at DATETIME,
		notes TEXT,
		is_completed BOOLEAN DEFAULT 0,
		order_index INTEGER,
		FOREIGN KEY (sequence_id) REFERENCES sequences(id) ON DELETE CASCADE
	);
	
	CREATE TABLE IF NOT EXISTS tg_state (
		integration_id INTEGER PRIMARY KEY,
		pts INTEGER,
		qts INTEGER,
		seq INTEGER,
		date INTEGER,
		FOREIGN KEY (integration_id) REFERENCES integrations(id) ON DELETE CASCADE
	);`

	log.Println("[DB] Verifying schema...")

	// Check if we need to migrate from old accounts table
	var hasIntegrations bool
	err = DB.Get(&hasIntegrations, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='integrations'")
	if err == nil && !hasIntegrations {
		log.Println("[DB] Old schema detected. Starting migration...")

		// 1. Rename old tables
		DB.MustExec("ALTER TABLE accounts RENAME TO accounts_old")
		DB.MustExec("ALTER TABLE contacts RENAME TO contacts_old")
		DB.MustExec("ALTER TABLE messages RENAME TO messages_old")

		// 2. Create new schema
		DB.MustExec(schema)

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
		DB.MustExec(schema)
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
}
