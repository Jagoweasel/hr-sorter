package web

import (
	"context"
	"github.com/gorilla/csrf"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/i18n"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/service"
	"hr-sorter/internal/streaming"
	"hr-sorter/internal/tgclient"
	"net/http"
	"os"
	"strings"
)

type Handler struct {
	rootCtx context.Context

	// Managers
	tgManager      *tgclient.Manager
	hhManager      *hhclient.Manager
	logBroadcaster *streaming.LogBroadcaster

	// Templates
	templates *TemplateManager
	i18n      *i18n.LocalizationService

	// Repositories
	accRepo *repository.AccountRepository
	intRepo *repository.IntegrationRepository
	conRepo *repository.ContactRepository
	msgRepo *repository.MessageRepository
	seqRepo *repository.SequenceRepository
	fltRepo *repository.FilterRepository
	mapRepo *repository.MappingRepository

	// Services
	accService *service.AccountService
	intService *service.IntegrationService
	seqService *service.SequenceService
	conService *service.ContactService
	fltService *service.FilterService
	repService *service.ReportService
}

func NewHandler(
	ctx context.Context,
	tgManager *tgclient.Manager,
	hhManager *hhclient.Manager,
	logBroadcaster *streaming.LogBroadcaster,
	templates *TemplateManager,
	ls *i18n.LocalizationService,
	accRepo *repository.AccountRepository,
	intRepo *repository.IntegrationRepository,
	conRepo *repository.ContactRepository,
	msgRepo *repository.MessageRepository,
	seqRepo *repository.SequenceRepository,
	fltRepo *repository.FilterRepository,
	mapRepo *repository.MappingRepository,
	accService *service.AccountService,
	intService *service.IntegrationService,
	seqService *service.SequenceService,
	conService *service.ContactService,
	fltService *service.FilterService,
	repService *service.ReportService,
) *Handler {
	return &Handler{
		rootCtx:        ctx,
		tgManager:      tgManager,
		hhManager:      hhManager,
		logBroadcaster: logBroadcaster,
		templates:      templates,
		i18n:           ls,
		accRepo:        accRepo,
		intRepo:        intRepo,
		conRepo:        conRepo,
		msgRepo:        msgRepo,
		seqRepo:        seqRepo,
		fltRepo:        fltRepo,
		mapRepo:        mapRepo,
		accService:     accService,
		intService:     intService,
		seqService:     seqService,
		conService:     conService,
		fltService:     fltService,
		repService:     repService,
	}
}

func (h *Handler) getLocale(r *http.Request) string {
	if cookie, err := r.Cookie("lang"); err == nil {
		return cookie.Value
	}
	return "en"
}

func (h *Handler) InitHandler() http.Handler {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	secret := os.Getenv("CSRF_SECRET")
	if len(secret) < 32 {
		secret = "32-byte-long-auth-key-hr-sorter-!" // Fallback for dev
	}

	csrfMiddleware := csrf.Protect(
		[]byte(secret),
		csrf.Secure(false), // Change to true in production with HTTPS
		csrf.Path("/"),
	)

	return csrfMiddleware(mux)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/reports", h.handleReports)
	mux.HandleFunc("/reports/export/xlsx", h.handleExportXLSX)
	mux.HandleFunc("/reports/export/pdf-options", h.handleExportPDFOptions)
	mux.HandleFunc("/reports/export/pdf", h.handleExportPDF)
	mux.HandleFunc("/contacts", h.handleContacts)
	mux.HandleFunc("/messages/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/send") {
			h.handleSendMessage(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "/list/") {
			h.handleMessageList(w, r)
			return
		}
		h.handleMessages(w, r)
	})
	mux.HandleFunc("/accounts", h.handleAccounts)
	mux.HandleFunc("/accounts/create", h.handleCreateAccount)
	mux.HandleFunc("/accounts/toggle", h.handleToggleAccount)
	mux.HandleFunc("/accounts/edit-modal", h.handleEditAccountModal)
	mux.HandleFunc("/accounts/update", h.handleUpdateAccount)
	mux.HandleFunc("/accounts/delete", h.handleDeleteAccount)
	mux.HandleFunc("/integrations/create", h.handleCreateIntegration)
	mux.HandleFunc("/integrations/toggle", h.handleToggleIntegration)
	mux.HandleFunc("/integrations/delete", h.handleDeleteIntegration)
	mux.HandleFunc("/integrations/status", h.handleIntegrationStatus)
	mux.HandleFunc("/integrations/submit-code", h.handleSubmitCode)
	mux.HandleFunc("/integrations/submit-password", h.handleSubmitPassword)
	mux.HandleFunc("/integrations/submit-hh-code", h.handleSubmitHHCode)
	mux.HandleFunc("/pipeline", h.handlePipeline)
	mux.HandleFunc("/contacts/", h.handleContactActions)
	mux.HandleFunc("/sequences/create", h.handleCreateSequence)
	mux.HandleFunc("/sequences/bulk-add", h.handleBulkAdd)
	mux.HandleFunc("/sequences/add-contact", h.handleAddToSequence)
	mux.HandleFunc("/sequences/edit-modal", h.handleEditSequenceModal)
	mux.HandleFunc("/sequences/update", h.handleUpdateSequence)
	mux.HandleFunc("/sequences/bulk-delete", h.handleBulkDeleteSequences)
	mux.HandleFunc("/sequences/add-stage-modal", h.handleAddStageModal)
	mux.HandleFunc("/stages/update", h.handleUpdateStage)
	mux.HandleFunc("/stages/add", h.handleAddStage)
	mux.HandleFunc("/sequences/move", h.handleMoveSequence)
	mux.HandleFunc("/sequences/delete", h.handleDeleteSequence)
	mux.HandleFunc("/filters", h.handleGetFilters)
	mux.HandleFunc("/filters/add", h.handleAddFilter)
	mux.HandleFunc("/filters/export", h.handleExportFilters)
	mux.HandleFunc("/filters/import", h.handleImportFilters)
	mux.HandleFunc("/filters/upload", h.handleUploadFilters)
	mux.HandleFunc("/filters/clear", h.handleClearFilters)
	mux.HandleFunc("/filters/delete", h.handleDeleteFilter)
	mux.HandleFunc("/filters/toggle", h.handleToggleFilter)
	mux.HandleFunc("/set-lang", h.handleSetLang)
	mux.HandleFunc("/logs", h.handleLogs)
	mux.HandleFunc("/logs/stream", h.handleLogStream)
	mux.HandleFunc("/mapping", h.handleMapping)
	mux.HandleFunc("/mapping/save", h.handleSaveMapping)
	mux.HandleFunc("/mapping/delete", h.handleDeleteMapping)
}

func (h *Handler) handleSetLang(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if lang != "en" && lang != "ru" {
		lang = "en"
	}
	http.SetCookie(w, &http.Cookie{
		Name:  "lang",
		Value: lang,
		Path:  "/",
	})
	w.Header().Set("HX-Refresh", "true")
}
