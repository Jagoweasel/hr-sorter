package web

import (
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
