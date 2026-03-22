package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"hr-sorter/internal/database"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/tgclient"
	"hr-sorter/internal/web"
)

func main() {
	debugSync := flag.Bool("debug-sync", false, "Enable debug logs for Telegram synchronization")
	debugAdd := flag.Bool("debug-add", false, "Enable debug logs for adding sequences")
	debugHistory := flag.Bool("debug-history", false, "Enable debug logs for sequence history and movement")
	debugAll := flag.Bool("debug-all", false, "Enable all debug logs")
	flag.Parse()

	if *debugAll || *debugSync {
		logger.Enable(logger.Sync)
	}
	if *debugAll || *debugAdd {
		logger.Enable(logger.AddSequence)
	}
	if *debugAll || *debugHistory {
		logger.Enable(logger.History)
	}

	log.Println("[Main] Starting application...")
	_ = godotenv.Load()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	log.Printf("[Main] Initializing database at %s...", dbPath)
	database.InitDB(dbPath)
	log.Println("[Main] Database initialized successfully.")

	apiIDStr := os.Getenv("API_ID")
	apiHash := os.Getenv("API_HASH")

	if apiIDStr == "" || apiHash == "" {
		log.Fatal("[Main] API_ID or API_HASH not found in .env")
	}

	apiID, _ := strconv.Atoi(apiIDStr)

	manager := tgclient.NewManager(apiID, apiHash)

	// Fetch active accounts from DB
	var accounts []models.Account
	err := database.DB.Select(&accounts, "SELECT * FROM accounts WHERE status = 'active'")
	if err != nil {
		log.Fatalf("[Main] Failed to fetch accounts: %v", err)
	}
	log.Printf("[Main] Found %d active accounts.", len(accounts))

	// Create root context for signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, acc := range accounts {
		go func(a models.Account) {
			if err := manager.StartAccount(ctx, a); err != nil {
				log.Printf("[Main] Account %s failed: %v", a.PhoneNumber, err)
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
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("[Main] Web server starting on http://127.0.0.1:%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Main] Web server failed: %s\n", err)
		}
	}()

	log.Println("[Main] Application is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	<-ctx.Done()
	stop()

	log.Println("[Main] Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Main] HTTP server Shutdown: %v", err)
	}

	log.Println("[Main] Shutdown complete.")
}
