package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"hr-sorter/internal/database"
	"hr-sorter/internal/tgclient"
	"hr-sorter/internal/web"
)

func main() {
	_ = godotenv.Load()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	database.InitDB(dbPath)

	apiIDStr := os.Getenv("API_ID")
	apiHash := os.Getenv("API_HASH")

	apiID, _ := strconv.Atoi(apiIDStr)

	manager := tgclient.NewManager(apiID, apiHash)
	_ = manager // Will be used to start accounts from DB

	mux := http.NewServeMux()
	web.RegisterRoutes(mux)

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on http://localhost:%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
