package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"hr-sorter/internal/database"
	"hr-sorter/internal/models"
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

	// Fetch active accounts from DB
	var accounts []models.Account
	err := database.DB.Select(&accounts, "SELECT * FROM accounts WHERE status = 'active'")
	if err != nil {
		log.Fatalf("failed to fetch accounts: %v", err)
	}

	// Create root context for signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, acc := range accounts {
		go func(a models.Account) {
			if err := manager.StartAccount(ctx, a); err != nil {
				log.Printf("Account %s failed: %v", a.PhoneNumber, err)
			}
		}(acc)
	}

	mux := http.NewServeMux()
	web.RegisterRoutes(mux)

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("Server starting on http://localhost:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	stop()

	log.Println("Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server Shutdown: %v", err)
	}

	log.Println("Shutdown complete.")
}
