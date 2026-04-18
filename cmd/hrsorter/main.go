package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"hr-sorter/internal/auth/hh"
	"hr-sorter/internal/database"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/i18n"
	internalLogger "hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/service"
	"hr-sorter/internal/streaming"
	"hr-sorter/internal/tgclient"
	"hr-sorter/internal/web"
	"hr-sorter/pkg/logger"
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
	debugTrace := flag.Bool("debug-trace", false, "Enable extreme detailed trace logs (network, playwright internal)")
	debugHHNet := flag.Bool("debug-hh-net", false, "Enable noisy network logs for HH (yandex, mail.ru, etc)")
	debugAll := flag.Bool("debug-all", false, "Enable all debug logs")
	flag.Parse()

	log.Println("[Main] Starting application...")
	_ = godotenv.Load()

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "data/hr-sorter.db"
	}

	// Ensure directory for database and sessions exists
	if dir := filepath.Dir(dbPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Fatalf("[Main] Failed to create database directory %s: %v", dir, err)
		}
	}
	if err := os.MkdirAll("data/sessions", 0755); err != nil {
		log.Fatalf("[Main] Failed to create sessions directory: %v", err)
	}

	log.Printf("[Main] Initializing database at %s...", dbPath)
	database.InitDB(dbPath)
	defer database.CloseDB()
	log.Println("[Main] Database initialized successfully.")

	// Initialize log streaming
	logBroadcaster := streaming.NewLogBroadcaster()

	// Initialize UNIFIED Zap Logger
	logger.L = logger.NewLogger(logBroadcaster, database.DB)
	log.SetOutput(io.MultiWriter(os.Stdout, logBroadcaster))

	if *debugAll || *debugSync {
		internalLogger.Enable(internalLogger.Sync)
	}
	if *debugAll || *debugAdd {
		internalLogger.Enable(internalLogger.AddSequence)
	}
	if *debugAll || *debugHistory {
		internalLogger.Enable(internalLogger.History)
	}
	if *debugAll || *debugTG {
		internalLogger.Enable(internalLogger.Telegram)
	}
	if *debugAll || *debugHH {
		internalLogger.Enable(internalLogger.HH)
	}
	if *debugAll || *debugReports {
		internalLogger.Enable(internalLogger.Reports)
	}
	if *debugAll || *debugMsg {
		internalLogger.Enable(internalLogger.Messaging)
	}
	if *debugAll || *debugFilters {
		internalLogger.Enable(internalLogger.Filters)
	}
	if *debugAll || *debugTrace {
		internalLogger.Enable(internalLogger.TraceCat)
	}
	if *debugAll || *debugHHNet {
		internalLogger.Enable(internalLogger.HHNet)
	}

	// Initialize repositories
	accRepo := repository.NewAccountRepository(database.DB)
	intRepo := repository.NewIntegrationRepository(database.DB)
	conRepo := repository.NewContactRepository(database.DB)
	msgRepo := repository.NewMessageRepository(database.DB)
	seqRepo := repository.NewSequenceRepository(database.DB)
	fltRepo := repository.NewFilterRepository(database.DB)
	mapRepo := repository.NewMappingRepository(database.DB)
	stRepo := repository.NewStateRepository(database.DB)

	manager := tgclient.NewManager(database.DB, conRepo, msgRepo, stRepo, intRepo, logBroadcaster)
	hhManager := hhclient.NewManager(database.DB, conRepo, msgRepo, intRepo, logBroadcaster)

	// Initialize HH Auth Module (New)
	hhConfig := hh.GetDefaultClientConfig()
	hhStorage := hh.NewSQLiteSessionStorage(database.DB)
	hhUAGen := hh.NewAndroidUserAgentGenerator()

	headless := os.Getenv("HEADLESS") != "false"
	log.Printf("[Main] Initializing HH Auth Authenticator (headless=%v)...", headless)

	hhAuthService, err := hh.NewPlaywrightAuthenticator(hhStorage, hhConfig, hhUAGen, headless)
	if err != nil {
		log.Printf("[Main] CRITICAL ERROR: Failed to initialize HH Auth Service: %v", err)
		log.Printf("[Main] Hint: Ensure Playwright browsers are installed: 'go run github.com/playwright-community/playwright-go/cmd/playwright install --with-deps'")
	} else if hhAuthService != nil {
		log.Println("[Main] HH Auth Service (Playwright) initialized successfully.")
		defer hhAuthService.Close()
	}

	// Initialize Localization
	ls, err := i18n.NewLocalizationService()
	if err != nil {
		log.Fatalf("[Main] Failed to initialize localization: %v", err)
	}

	// Initialize Template Manager
	tm, err := web.NewTemplateManager(ls)
	if err != nil {
		log.Fatalf("[Main] Failed to initialize templates: %v", err)
	}

	// Initialize services
	accService := service.NewAccountService(accRepo, intRepo, manager, hhManager)
	intService := service.NewIntegrationService(intRepo, manager, hhManager, hhAuthService)
	conService := service.NewContactService(conRepo, fltRepo, msgRepo, manager, hhManager)
	seqService := service.NewSequenceService(seqRepo, conRepo, accRepo, conService)
	fltService := service.NewFilterService(fltRepo)
	repService := service.NewReportService(seqRepo, accRepo, mapRepo)

	// Fetch active TG/HH integrations from active accounts
	integrations, err := intRepo.GetActiveAndPending(context.Background())
	if err != nil {
		log.Fatalf("[Main] Failed to fetch integrations: %v", err)
	}
	log.Printf("[Main] Found %d active integrations.", len(integrations))

	// Create root context for signal handling
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup

	for _, integration := range integrations {
		if integration.Platform == "tg" {
			wg.Add(1)
			go func(i models.Integration) {
				defer wg.Done()
				if err := manager.StartIntegration(ctx, i); err != nil {
					log.Printf("[Main] TG Integration %s failed: %v", i.Identifier, err)
				}
			}(integration)
		} else if integration.Platform == "hh" {
			wg.Add(1)
			go func(i models.Integration) {
				defer wg.Done()
				if err := hhManager.StartIntegration(ctx, i); err != nil {
					log.Printf("[Main] HH Integration %s failed: %v", i.Identifier, err)
				}
			}(integration)
		}
	}

	handler := web.NewHandler(ctx, manager, hhManager, hhAuthService, logBroadcaster, tm, ls, accRepo, intRepo, conRepo, msgRepo, seqRepo, fltRepo, mapRepo, accService, intService, seqService, conService, fltService, repService, database.DB)
	wrappedHandler := handler.InitHandler()

	port := os.Getenv("HTTP_PORT")
	if port == "" {
		port = "3000"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: wrappedHandler,
	}

	go func() {
		log.Printf("[Main] Web server starting on port %s (accessible at http://localhost:%s)", port, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Main] Web server failed: %s\n", err)
		}
	}()

	log.Println("[Main] Application is running. Press Ctrl+C to stop.")

	// Wait for interrupt signal
	<-ctx.Done()
	log.Println("[Main] Shutting down gracefully...")

	// Shutdown HTTP server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[Main] HTTP server Shutdown: %v", err)
	}

	log.Println("[Main] Waiting for background integrations to stop...")
	wg.Wait()

	log.Println("[Main] Shutdown complete.")
}
