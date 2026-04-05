package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"hr-sorter/internal/database"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/i18n"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/service"
	"hr-sorter/internal/streaming"
	"hr-sorter/internal/tgclient"
	"hr-sorter/internal/web"
)

func main() {
	debugSync := flag.Bool("debug-sync", false, "Enable debug logs for Telegram synchronization")
	debugAdd := flag.Bool("debug-add", false, "Enable debug logs for adding sequences")
	debugHistory := flag.Bool("debug-history", false, "Enable debug logs for sequence history and movement")
	debugTG := flag.Bool("debug-tg", false, "Enable debug logs for Telegram API and client creation")
	debugHH := flag.Bool("debug-hh", false, "Enable debug logs for HeadHunter API and sync")
	debugReports := flag.Bool("debug-reports", false, "Enable debug logs for report generation")
	debugMsg := flag.Bool("debug-msg", false, "Enable debug logs for messaging")
	debugFilters := flag.Bool("debug-filters", false, "Enable debug logs for message filtering")
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
	if *debugAll || *debugHH {
		logger.Enable(logger.HH)
	}
	if *debugAll || *debugReports {
		logger.Enable(logger.Reports)
	}
	if *debugAll || *debugMsg {
		logger.Enable(logger.Messaging)
	}
	if *debugAll || *debugFilters {
		logger.Enable(logger.Filters)
	}

	log.Println("[Main] Starting application...")
	_ = godotenv.Load()

	// Initialize log streaming
	logBroadcaster := streaming.NewLogBroadcaster()
	log.SetOutput(io.MultiWriter(os.Stdout, logBroadcaster))

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	log.Printf("[Main] Initializing database at %s...", dbPath)
	database.InitDB(dbPath)
	log.Println("[Main] Database initialized successfully.")

	// Initialize repositories
	accRepo := repository.NewAccountRepository(database.DB)
	intRepo := repository.NewIntegrationRepository(database.DB)
	conRepo := repository.NewContactRepository(database.DB)
	msgRepo := repository.NewMessageRepository(database.DB)
	seqRepo := repository.NewSequenceRepository(database.DB)
	fltRepo := repository.NewFilterRepository(database.DB)
	mapRepo := repository.NewMappingRepository(database.DB)
	stRepo := repository.NewStateRepository(database.DB)

	manager := tgclient.NewManager(database.DB, conRepo, msgRepo, stRepo, intRepo)
	hhManager := hhclient.NewManager(database.DB, conRepo, msgRepo, intRepo)

	// Initialize Localization
	ls := i18n.NewLocalizationService()

	// Initialize Template Manager
	tm, err := web.NewTemplateManager(ls)
	if err != nil {
		log.Fatalf("[Main] Failed to initialize templates: %v", err)
	}

	// Initialize services
	accService := service.NewAccountService(accRepo, intRepo, manager, hhManager)
	intService := service.NewIntegrationService(intRepo, manager, hhManager)
	conService := service.NewContactService(conRepo, fltRepo, msgRepo, manager, hhManager)
	seqService := service.NewSequenceService(seqRepo, conRepo, accRepo, conService)
	fltService := service.NewFilterService(fltRepo)
	repService := service.NewReportService(seqRepo, accRepo)

	// Fetch active TG/HH integrations from active accounts
	integrations, err := intRepo.GetActiveAndPending(context.Background())
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
	handler := web.NewHandler(ctx, manager, hhManager, logBroadcaster, tm, ls, accRepo, intRepo, conRepo, msgRepo, seqRepo, fltRepo, mapRepo, accService, intService, seqService, conService, fltService, repService)
	handler.RegisterRoutes(mux)

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
