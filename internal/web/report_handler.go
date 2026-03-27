package web

import (
	"hr-sorter/internal/service"
	"log"
	"net/http"
)

func (h *Handler) handleReports(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")

	allAccounts, err := h.accRepo.GetAll(r.Context())
	if err != nil {
		log.Printf("Web: Error fetching accounts: %v", err)
	}

	reportData, err := h.repService.GetReportData(r.Context(), accountID)
	if err != nil {
		log.Printf("Web: Error fetching report data: %v", err)
		http.Error(w, "Error generating report", 500)
		return
	}

	dataInterface := struct {
		Accounts  interface{}
		ActiveAcc string
		Report    interface{}
	}{
		Accounts:  allAccounts,
		ActiveAcc: accountID,
		Report:    reportData,
	}

	h.templates.RenderWithStatus(w, "reports.html", http.StatusOK, dataInterface)
}

func (h *Handler) handleExportXLSX(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")

	data, err := h.repService.ExportSummarizedXLSX(r.Context(), accountID)
	if err != nil {
		log.Printf("Web: Error exporting XLSX: %v", err)
		http.Error(w, "Error generating export", 500)
		return
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", "attachment; filename=recruitment_report.xlsx")
	w.Write(data)
}

func (h *Handler) handleExportPDFOptions(w http.ResponseWriter, r *http.Request) {
	accountID := r.URL.Query().Get("account_id")
	h.templates.RenderWithStatus(w, "fragments/modals/export_pdf_options.html", http.StatusOK, map[string]string{"AccountID": accountID})
}

func (h *Handler) handleExportPDF(w http.ResponseWriter, r *http.Request) {
	accountID := r.FormValue("account_id")
	opts := service.PDFExportOptions{
		IncludeKPIs:      r.FormValue("include_kpis") == "on",
		IncludeFunnel:    r.FormValue("include_funnel") == "on",
		IncludeVacancies: r.FormValue("include_vacancies") == "on",
		IncludeCompanies: r.FormValue("include_companies") == "on",
		IncludeDetailed:  r.FormValue("include_detailed") == "on",
		IncludeTimeline:  r.FormValue("include_timeline") == "on",
	}

	data, err := h.repService.ExportPDF(r.Context(), accountID, opts)
	if err != nil {
		log.Printf("Web: Error exporting PDF: %v", err)
		http.Error(w, "Error generating export", 500)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename=recruitment_report.pdf")
	w.Write(data)
}
