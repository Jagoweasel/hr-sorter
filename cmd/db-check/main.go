package main

import (
	"fmt"
	"log"
	"os"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	db, err := sqlx.Connect("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count int
	err = db.Get(&count, "SELECT count(*) FROM contacts")
	fmt.Printf("Contacts: %d (err: %v)\n", count, err)

	err = db.Get(&count, "SELECT count(*) FROM messages")
	fmt.Printf("Messages: %d (err: %v)\n", count, err)

	type Contact struct {
		ID       int64  `db:"id"`
		Username string `db:"username"`
	}
	var contacts []Contact
	err = db.Select(&contacts, "SELECT id, username FROM contacts LIMIT 5")
	if err != nil {
		fmt.Printf("Error selecting contacts: %v\n", err)
	}
	for _, c := range contacts {
		fmt.Printf(" - Contact ID %d: @%s\n", c.ID, c.Username)
	}
}
