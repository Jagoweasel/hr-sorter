package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"hr-sorter/internal/database"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/tgclient"
	"hr-sorter/internal/web"
)

func main() {
	debugSync := flag.Bool("debug-sync", false, "Enable debug logs for Telegram synchronization")
	debugAdd := flag.Bool("debug-add", false, "Enable debug logs for adding sequences")
	debugHistory := flag.Bool("debug-history", false, "Enable debug logs for sequence history and movement")
	debugTG := flag.Bool("debug-tg", false, "Enable debug logs for Telegram API and client creation")
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
	if *debugAll || *debugTG {
		logger.Enable(logger.Telegram)
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

	manager := tgclient.NewManager()
	hhManager := hhclient.NewManager()

	// Fetch active TG/HH integrations from active accounts
	var integrations []models.Integration
	err := database.DB.Select(&integrations, `
		SELECT i.* FROM integrations i
		JOIN accounts a ON i.account_id = a.id
		WHERE i.status IN ('active', 'pending_auth') AND a.status = 'active'
	`)
	if err != nil {
		log.Fatalf("[Main] Failed to fetch integrations: %v", err)
	}
	log.Printf("[Main] Found %d active integrations.", len(integrations))

	// Create root context for signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	for _, integration := range integrations {
		if integration.Platform == "tg" {
			go func(i models.Integration) {
				if err := manager.StartIntegration(ctx, i); err != nil {
					log.Printf("[Main] TG Integration %s failed: %v", i.Identifier, err)
				}
			}(integration)
		} else if integration.Platform == "hh" {
			go func(i models.Integration) {
				if err := hhManager.StartIntegration(ctx, i); err != nil {
					log.Printf("[Main] HH Integration %s failed: %v", i.Identifier, err)
				}
			}(integration)
		}
	}

	mux := http.NewServeMux()
	web.RegisterRoutes(mux, manager, ctx)

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
