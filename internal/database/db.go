package database

import (
	"log"
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
		dsn = path + "?_pragma=foreign_keys=1&_journal_mode=WAL"
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
		phone_number TEXT UNIQUE NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending_auth',
		session_path TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS contacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		tg_user_id INTEGER UNIQUE NOT NULL,
		first_name TEXT,
		last_name TEXT,
		username TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER,
		contact_id INTEGER,
		tg_message_id INTEGER,
		text TEXT,
		is_incoming BOOLEAN,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(account_id, contact_id, tg_message_id),
		FOREIGN KEY (account_id) REFERENCES accounts(id),
		FOREIGN KEY (contact_id) REFERENCES contacts(id)
	);

	CREATE TABLE IF NOT EXISTS sequences (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		account_id INTEGER,
		company_name TEXT NOT NULL,
		vacancy_name TEXT NOT NULL,
		status TEXT DEFAULT 'initial',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (account_id) REFERENCES accounts(id)
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
	);`

	log.Println("[DB] Verifying schema...")
	DB.MustExec(schema)
	log.Println("[DB] Schema verified.")

	// Migration: Add tg_message_id if it doesn't exist
	var hasColumn bool
	err = DB.Get(&hasColumn, "SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='tg_message_id'")
	if err == nil && !hasColumn {
		log.Println("[DB] Migrating database: adding tg_message_id to messages table...")
		DB.MustExec("ALTER TABLE messages ADD COLUMN tg_message_id INTEGER")
		DB.MustExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_unique_sync ON messages(account_id, contact_id, tg_message_id)")
		log.Println("[DB] Migration finished.")
	}

	// Migration: Add account_id to sequences if it doesn't exist
	err = DB.Get(&hasColumn, "SELECT COUNT(*) FROM pragma_table_info('sequences') WHERE name='account_id'")
	if err == nil && !hasColumn {
		log.Println("[DB] Migrating database: adding account_id to sequences table...")
		DB.MustExec("ALTER TABLE sequences ADD COLUMN account_id INTEGER REFERENCES accounts(id)")
		log.Println("[DB] Migration finished.")
	}
}
