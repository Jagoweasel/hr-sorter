package main

import (
	"context"
	"fmt"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/i18n"
	"hr-sorter/internal/mapping"
	"hr-sorter/internal/report"
	"hr-sorter/internal/storage"
	"hr-sorter/internal/streaming"
	"log"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// 1. Initialize DB with SQLite tuning
	repo, err := storage.NewSQLiteRepository("hr-sorter.db")
	if err != nil {
		log.Fatal(err)
	}

	// 2. Initialize i18n
	translator, err := i18n.NewLocalizationService()
	if err != nil {
		log.Fatal(err)
	}
	if err := translator.Load("en", "ru"); err != nil {
		log.Fatal(err)
	}

	// 3. Initialize HeadHunter Service (Playwright)
	hhService, err := hhclient.NewHHAuthService(repo)
	if err != nil {
		log.Fatal(err)
	}
	defer hhService.Close()
	_ = hhService // To be used by API handlers

	// 4. Initialize Reporting Engine (maroto/v2)
	reporter := report.NewPDFReportService()
	_ = reporter

	// 5. Initialize Vacancy Mapping
	rules, _ := repo.GetMappingRules(ctx)
	mapper := mapping.NewRegexMapper()
	mapper.UpdateRules(ctx, rules)
	_ = mapper

	// 6. Initialize Log Streaming (zap broadcaster)
	logBroadcaster := streaming.NewLogBroadcaster()
	_ = logBroadcaster
	// Use logBroadcaster as a zap.WriteSyncer or as an io.Writer

	// 7. Start API/Web Server (Echo/Gin)
	fmt.Println("HR-SORTER v2.0 Starting...")

	// Wait for shutdown
	<-ctx.Done()
	fmt.Println("Shutting down...")
}
