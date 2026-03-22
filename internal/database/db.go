package database

import (
	"log"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var DB *sqlx.DB

func InitDB(path string) {
	var err error
	dsn := path
	if !strings.Contains(path, "?") {
		dsn = path + "?_pragma=foreign_keys=1&_journal_mode=WAL"
	}
	DB, err = sqlx.Connect("sqlite", dsn)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

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
	);`

	DB.MustExec(schema)

	// Migration: Add tg_message_id if it doesn't exist
	var hasColumn bool
	err = DB.Get(&hasColumn, "SELECT COUNT(*) FROM pragma_table_info('messages') WHERE name='tg_message_id'")
	if err == nil && !hasColumn {
		log.Println("Migrating database: adding tg_message_id to messages table...")
		DB.MustExec("ALTER TABLE messages ADD COLUMN tg_message_id INTEGER")
		// We can't easily add UNIQUE constraint via ALTER in SQLite,
		// but we can add a UNIQUE index.
		DB.MustExec("CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_unique_sync ON messages(account_id, contact_id, tg_message_id)")
	}
}
