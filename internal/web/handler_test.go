package web

import (
	"context"
	"hr-sorter/internal/database"
	"hr-sorter/internal/i18n"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/service"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGETEndpoints(t *testing.T) {
	// Go to project root to find templates and locales
	originalWD, _ := os.Getwd()
	os.Chdir("../../")
	defer os.Chdir(originalWD)

	// 1. Setup in-memory DB
	database.InitDB(":memory:")
	db := database.DB

	// 2. Setup Repositories
	accRepo := repository.NewAccountRepository(db)
	intRepo := repository.NewIntegrationRepository(db)
	conRepo := repository.NewContactRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	seqRepo := repository.NewSequenceRepository(db)
	fltRepo := repository.NewFilterRepository(db)
	mapRepo := repository.NewMappingRepository(db)

	// 3. Setup Services
	accService := service.NewAccountService(accRepo, intRepo, nil, nil)
	intService := service.NewIntegrationService(intRepo, nil, nil)
	conService := service.NewContactService(conRepo, fltRepo, msgRepo, nil, nil)
	seqService := service.NewSequenceService(seqRepo, conRepo, accRepo, conService)
	fltService := service.NewFilterService(fltRepo)
	repService := service.NewReportService(seqRepo, accRepo, mapRepo)

	// 4. Setup I18n & Templates
	ls, err := i18n.NewLocalizationService()
	if err != nil {
		t.Fatalf("failed to init i18n: %v", err)
	}
	tm, err := NewTemplateManager(ls)
	if err != nil {
		t.Fatalf("failed to init templates: %v", err)
	}

	// 5. Create Handler
	ctx := context.Background()
	h := NewHandler(ctx, nil, nil, nil, nil, tm, ls,
		accRepo, intRepo, conRepo, msgRepo, seqRepo, fltRepo, mapRepo,
		accService, intService, seqService, conService, fltService, repService)

	// Seed some data
	accRepo.Create(ctx, "Test Account", "test-account")

	tests := []struct {
		name           string
		url            string
		expectedStatus int
		expectedText   string // Translated text to check
		lang           string
	}{
		{
			name:           "Accounts EN",
			url:            "/accounts",
			expectedStatus: http.StatusOK,
			expectedText:   "Account Management",
			lang:           "en",
		},
		{
			name:           "Accounts RU",
			url:            "/accounts",
			expectedStatus: http.StatusOK,
			expectedText:   "Управление аккаунтами",
			lang:           "ru",
		},
		{
			name:           "Pipeline EN",
			url:            "/pipeline",
			expectedStatus: http.StatusOK,
			expectedText:   "Pipeline",
			lang:           "en",
		},
		{
			name:           "Mapping EN",
			url:            "/mapping",
			expectedStatus: http.StatusOK,
			expectedText:   "Vacancy Mapping",
			lang:           "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, _ := http.NewRequest("GET", tt.url, nil)
			if tt.lang != "" {
				req.AddCookie(&http.Cookie{Name: "lang", Value: tt.lang})
			}
			rr := httptest.NewRecorder()

			// Use the handler directly for GET requests
			var handler http.HandlerFunc
			switch tt.url {
			case "/accounts":
				handler = h.handleAccounts
			case "/pipeline":
				handler = h.handlePipeline
			case "/mapping":
				handler = h.handleMapping
			default:
				t.Fatalf("Unknown URL: %s", tt.url)
			}

			handler.ServeHTTP(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, rr.Code)
			}

			if !strings.Contains(rr.Body.String(), tt.expectedText) {
				t.Errorf("expected body to contain %q", tt.expectedText)
			}
		})
	}
}
