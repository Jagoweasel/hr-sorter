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

	var accCount int
	err = db.Get(&accCount, "SELECT count(*) FROM accounts")
	fmt.Printf("Accounts: %d (err: %v)\n", accCount, err)

	type Acc struct {
		ID     int64  `db:"id"`
		Status string `db:"status"`
	}
	var accs []Acc
	err = db.Select(&accs, "SELECT id, status FROM accounts")
	if err == nil {
		for _, a := range accs {
			fmt.Printf(" - Acc ID %d: %s\n", a.ID, a.Status)
		}
	}

	var seqCount int
	err = db.Get(&seqCount, "SELECT count(*) FROM sequences")
	fmt.Printf("Sequences: %d (err: %v)\n", seqCount, err)

	type Seq struct {
		ID          int64  `db:"id"`
		CompanyName string `db:"company_name"`
		AccountID   *int64 `db:"account_id"`
	}
	var seqs []Seq
	err = db.Select(&seqs, "SELECT id, company_name, account_id FROM sequences LIMIT 5")
	if err != nil {
		fmt.Printf("Error selecting sequences: %v\n", err)
	}
	for _, s := range seqs {
		accID := "NULL"
		if s.AccountID != nil {
			accID = fmt.Sprintf("%d", *s.AccountID)
		}
		fmt.Printf(" - Seq ID %d: %s (AccountID: %s)\n", s.ID, s.CompanyName, accID)
	}

	var columns []struct {
		Name string `db:"name"`
	}
	db.Select(&columns, "PRAGMA table_info(sequences)")
	fmt.Printf("Sequences columns: ")
	for _, col := range columns {
		fmt.Printf("%s ", col.Name)
	}
	fmt.Println()

	columns = nil
	db.Select(&columns, "PRAGMA table_info(interview_stages)")
	fmt.Printf("InterviewStages columns: ")
	for _, col := range columns {
		fmt.Printf("%s ", col.Name)
	}
	fmt.Println()

	var stagesCount int
	err = db.Get(&stagesCount, "SELECT count(*) FROM interview_stages")
	fmt.Printf("InterviewStages: %d (err: %v)\n", stagesCount, err)

	type Stage struct {
		ID          int64  `db:"id"`
		SequenceID  int64  `db:"sequence_id"`
		Name        string `db:"name"`
		IsCompleted bool   `db:"is_completed"`
	}
	var stages_list []Stage
	err = db.Select(&stages_list, "SELECT id, sequence_id, name, is_completed FROM interview_stages LIMIT 10")
	if err == nil {
		for _, s := range stages_list {
			fmt.Printf(" - Stage ID %d (Seq %d): %s (Completed: %v)\n", s.ID, s.SequenceID, s.Name, s.IsCompleted)
		}
	}
}
