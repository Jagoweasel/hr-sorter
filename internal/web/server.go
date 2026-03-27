package web

import (
	"context"
	"hr-sorter/internal/hhclient"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/service"
	"hr-sorter/internal/tgclient"
	"net/http"
)

type Handler struct {
	rootCtx context.Context

	// Managers
	tgManager *tgclient.Manager
	hhManager *hhclient.Manager

	// Templates
	templates *TemplateManager

	// Repositories
	accRepo *repository.AccountRepository
	intRepo *repository.IntegrationRepository
	conRepo *repository.ContactRepository
	msgRepo *repository.MessageRepository
	seqRepo *repository.SequenceRepository
	fltRepo *repository.FilterRepository

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
	templates *TemplateManager,
	accRepo *repository.AccountRepository,
	intRepo *repository.IntegrationRepository,
	conRepo *repository.ContactRepository,
	msgRepo *repository.MessageRepository,
	seqRepo *repository.SequenceRepository,
	fltRepo *repository.FilterRepository,
	accService *service.AccountService,
	intService *service.IntegrationService,
	seqService *service.SequenceService,
	conService *service.ContactService,
	fltService *service.FilterService,
	repService *service.ReportService,
) *Handler {
	return &Handler{
		rootCtx:    ctx,
		tgManager:  tgManager,
		hhManager:  hhManager,
		templates:  templates,
		accRepo:    accRepo,
		intRepo:    intRepo,
		conRepo:    conRepo,
		msgRepo:    msgRepo,
		seqRepo:    seqRepo,
		fltRepo:    fltRepo,
		accService: accService,
		intService: intService,
		seqService: seqService,
		conService: conService,
		fltService: fltService,
		repService: repService,
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/reports", h.handleReports)
	mux.HandleFunc("/reports/export/xlsx", h.handleExportXLSX)
	mux.HandleFunc("/reports/export/pdf-options", h.handleExportPDFOptions)
	mux.HandleFunc("/reports/export/pdf", h.handleExportPDF)
	mux.HandleFunc("/contacts", h.handleContacts)
	mux.HandleFunc("/messages/", h.handleMessages)
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
	mux.HandleFunc("/filters/delete", h.handleDeleteFilter)
	mux.HandleFunc("/filters/toggle", h.handleToggleFilter)
}
