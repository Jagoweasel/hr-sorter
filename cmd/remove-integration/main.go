package main

import (
	"bufio"
	"fmt"
	"log"
	"os"

	"hr-sorter/internal/database"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	database.InitDB(dbPath)

	type IntegrationRow struct {
		ID          int64  `db:"id"`
		AccountID   int64  `db:"account_id"`
		Platform    string `db:"platform"`
		Identifier  string `db:"identifier"`
		Status      string `db:"status"`
		SessionPath string `db:"session_path"`
	}

	var integrations []IntegrationRow
	err := database.DB.Select(&integrations, "SELECT id, account_id, platform, identifier, status, session_path FROM integrations ORDER BY id")
	if err != nil {
		log.Fatal("Failed to fetch integrations:", err)
	}

	fmt.Println("=== Existing Integrations ===")
	if len(integrations) == 0 {
		fmt.Println("No integrations found.")
		return
	}

	for _, i := range integrations {
		fmt.Printf("[%d] %s (%s) - %s\n", i.ID, i.Identifier, i.Platform, i.Status)
		if i.SessionPath != "" {
			fmt.Printf("     Session: %s\n", i.SessionPath)
		}
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter ID to remove: ")
	idStr, _ := reader.ReadString('\n')
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	if id == 0 {
		log.Fatal("Invalid ID")
	}

	// Get session path before deletion
	var sessionPath string
	database.DB.Get(&sessionPath, "SELECT session_path FROM integrations WHERE id = ?", id)

	// Delete from DB
	_, err = database.DB.Exec("DELETE FROM integrations WHERE id = ?", id)
	if err != nil {
		log.Fatal("Failed to delete integration:", err)
	}

	// Delete session file if exists
	if sessionPath != "" {
		if _, err := os.Stat(sessionPath); err == nil {
			if err := os.Remove(sessionPath); err != nil {
				log.Printf("Warning: failed to delete session file: %v", err)
			} else {
				fmt.Printf("Deleted session file: %s\n", sessionPath)
			}
		}
	}

	fmt.Printf("\nSuccess! Integration ID %d removed.\n", id)
}
