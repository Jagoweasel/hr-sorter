package web

import (
	"context"
	"github.com/gorilla/csrf"
	"github.com/jmoiron/sqlx"
	"hr-sorter/internal/auth/hh"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/i18n"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
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
	hhAuthService  hh.Authenticator
	logBroadcaster *streaming.LogBroadcaster

	// Templates
	templates *TemplateManager
	i18n      *i18n.LocalizationService

	db *sqlx.DB

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
	hhAuthService hh.Authenticator,
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
	db *sqlx.DB,
) *Handler {
	return &Handler{
		rootCtx:        ctx,
		tgManager:      tgManager,
		hhManager:      hhManager,
		hhAuthService:  hhAuthService,
		logBroadcaster: logBroadcaster,
		templates:      templates,
		i18n:           ls,
		db:             db,
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

func (h *Handler) handleCSRFError(w http.ResponseWriter, r *http.Request) {
	reason := csrf.FailureReason(r)
	logger.Error(logger.HH, "CSRF Error: %v | Method: %s | URL: %s | Origin: %s | Referer: %s",
		reason, r.Method, r.URL.String(), r.Header.Get("Origin"), r.Header.Get("Referer"))
	http.Error(w, "Forbidden - "+reason.Error(), http.StatusForbidden)
}

func (h *Handler) getCookie(r *http.Request, name, defaultValue string) string {
	cookie, err := r.Cookie(name)
	if err == nil {
		return cookie.Value
	}
	return defaultValue
}

func (h *Handler) setHXLocation(w http.ResponseWriter, path string) {
	// We use JSON format for HX-Location to specify the target container and morph swap.
	// This prevents HTMX from replacing the whole body and losing the navigation header,
	// while also preserving scroll position.
	w.Header().Set("HX-Location", "{\"path\":\""+path+"\", \"target\":\"#main-content\", \"swap\":\"morph\"}")
	// Signal to close any open modal
	w.Header().Set("HX-Trigger", "closeModal")
}

func (h *Handler) getColumnDefs(r *http.Request) []models.ColumnDef {
	locale := h.getLocale(r)
	return []models.ColumnDef{
		{ID: "initial", Label: h.i18n.Tr("initial", locale), ColorClass: "bg-blue-50", BorderClass: "border-blue-200"},
		{ID: "screening", Label: h.i18n.Tr("screening", locale), ColorClass: "bg-indigo-50", BorderClass: "border-indigo-200"},
		{ID: "tech", Label: h.i18n.Tr("technical", locale), ColorClass: "bg-purple-50", BorderClass: "border-purple-200"},
		{ID: "final", Label: h.i18n.Tr("final_interview", locale), ColorClass: "bg-pink-50", BorderClass: "border-pink-200"},
		{ID: "offer", Label: h.i18n.Tr("offer", locale), ColorClass: "bg-yellow-50", BorderClass: "border-yellow-200"},
		{ID: "accepted", Label: h.i18n.Tr("accepted", locale), ColorClass: "bg-green-50", BorderClass: "border-green-200"},
		{ID: "rejected", Label: h.i18n.Tr("rejected", locale), ColorClass: "bg-red-50", BorderClass: "border-red-200"},
	}
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
		csrf.TrustedOrigins([]string{"localhost:3000", "127.0.0.1:3000"}),
		csrf.ErrorHandler(http.HandlerFunc(h.handleCSRFError)),
	)

	return csrfMiddleware(mux)
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

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
	mux.HandleFunc("/integrations/start-auth", h.handleStartAuth)
	mux.HandleFunc("/integrations/reset-auth", h.handleResetHHAuth)
	mux.HandleFunc("/integrations/status", h.handleIntegrationStatus)
	mux.HandleFunc("/integrations/stats", h.handleIntegrationStats)
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
	mux.HandleFunc("/logs/update", h.handleUpdateLogConfig)
	mux.HandleFunc("/logs/history", h.handleLogHistory)
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
