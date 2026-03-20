package database

import (
	"log"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

var DB *sqlx.DB

func InitDB(path string) {
	var err error
	DB, err = sqlx.Connect("sqlite", path)
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
		text TEXT,
		is_incoming BOOLEAN,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (account_id) REFERENCES accounts(id),
		FOREIGN KEY (contact_id) REFERENCES contacts(id)
	);`

	DB.MustExec(schema)
}
