package main

import (
	"fmt"
	"log"
	"os"

	"hr-sorter/internal/database"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/hr-sorter.db"
	}
	database.InitDB(dbPath)

	confirm := ""
	fmt.Print("This will delete ALL contacts and messages. Are you sure? (y/N): ")
	fmt.Scanln(&confirm)

	if confirm != "y" && confirm != "Y" {
		fmt.Println("Aborted.")
		return
	}

	_, err := database.DB.Exec("DELETE FROM messages")
	if err != nil {
		log.Fatal(err)
	}
	_, err = database.DB.Exec("DELETE FROM contacts")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Database purged successfully (accounts kept).")
}
