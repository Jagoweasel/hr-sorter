package web

import (
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"log"
	"net/http"
)

type ColumnDef struct {
	ID          string
	Label       string
	ColorClass  string
	BorderClass string
}

type AccountGroup struct {
	Account   *models.Account
	Columns   []PipelineColumn
	Sequences []repository.SequenceWithDetails
}

type PipelineColumn struct {
	ColumnDef
	Sequences []repository.SequenceWithDetails
}

func (h *Handler) handlePipeline(w http.ResponseWriter, r *http.Request) {
	view := r.URL.Query().Get("view")
	if view == "" {
		view = "kanban"
	}

	activeAccountID := r.URL.Query().Get("account_id")
	hideRejected := r.URL.Query().Get("hide_rejected") == "true"
	hideAccepted := r.URL.Query().Get("hide_accepted") == "true"

	allAccounts, _ := h.accRepo.GetAll(r.Context())
	sequences, err := h.seqRepo.GetAll(r.Context(), activeAccountID)
	if err != nil {
		log.Printf("Web: Error fetching sequences: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	var detailedSeqs []repository.SequenceWithDetails
	for _, s := range sequences {
		if hideRejected && s.Status == "rejected" {
			continue
		}
		if hideAccepted && s.Status == "accepted" {
			continue
		}

		recruiters, _ := h.seqRepo.GetRecruiters(r.Context(), s.ID)
		stages, _ := h.seqRepo.GetStages(r.Context(), s.ID)

		var history []models.InterviewStage
		var historyNames []string
		for _, st := range stages {
			if st.IsCompleted {
				history = append(history, st)
				historyNames = append(historyNames, st.Name)
			}
		}

		detailedSeqs = append(detailedSeqs, repository.SequenceWithDetails{
			Sequence:   s,
			Recruiters: recruiters,
			Stages:     stages,
			History:    history,
			IsRejected: s.Status == "rejected",
			IsAccepted: s.Status == "accepted",
		})
		logger.LogChain(s.ID, s.CompanyName, historyNames, s.Status)
	}

	columnDefs := []ColumnDef{
		{ID: "initial", Label: h.i18n.Tr("initial", h.getLocale(r)), ColorClass: "bg-blue-50", BorderClass: "border-blue-200"},
		{ID: "screening", Label: h.i18n.Tr("screening", h.getLocale(r)), ColorClass: "bg-indigo-50", BorderClass: "border-indigo-200"},
		{ID: "tech", Label: h.i18n.Tr("technical", h.getLocale(r)), ColorClass: "bg-purple-50", BorderClass: "border-purple-200"},
		{ID: "final", Label: h.i18n.Tr("final_interview", h.getLocale(r)), ColorClass: "bg-pink-50", BorderClass: "border-pink-200"},
		{ID: "offer", Label: h.i18n.Tr("offer", h.getLocale(r)), ColorClass: "bg-yellow-50", BorderClass: "border-yellow-200"},
		{ID: "accepted", Label: h.i18n.Tr("accepted", h.getLocale(r)), ColorClass: "bg-green-50", BorderClass: "border-green-200"},
		{ID: "rejected", Label: h.i18n.Tr("rejected", h.getLocale(r)), ColorClass: "bg-red-50", BorderClass: "border-red-200"},
	}

	var groups []*AccountGroup
	orphanGroup := &AccountGroup{Account: nil}
	for _, def := range columnDefs {
		orphanGroup.Columns = append(orphanGroup.Columns, PipelineColumn{ColumnDef: def})
	}

	accountMap := make(map[int64]*AccountGroup)
	for i := range allAccounts {
		acc := allAccounts[i]
		group := &AccountGroup{Account: &acc}
		for _, def := range columnDefs {
			group.Columns = append(group.Columns, PipelineColumn{ColumnDef: def})
		}
		groups = append(groups, group)
		accountMap[acc.ID] = group
	}

	for _, s := range detailedSeqs {
		var targetGroup *AccountGroup
		if s.AccountID != nil {
			targetGroup = accountMap[*s.AccountID]
		}
		if targetGroup == nil {
			targetGroup = orphanGroup
		}

		targetGroup.Sequences = append(targetGroup.Sequences, s)
		for i := range targetGroup.Columns {
			if s.Status == targetGroup.Columns[i].ID {
				targetGroup.Columns[i].Sequences = append(targetGroup.Columns[i].Sequences, s)
			}
		}
	}

	if len(orphanGroup.Sequences) > 0 {
		groups = append(groups, orphanGroup)
	}

	data := struct {
		View         string
		Groups       []*AccountGroup
		ColumnDefs   []ColumnDef
		HideRejected bool
		HideAccepted bool
	}{
		View:         view,
		Groups:       groups,
		ColumnDefs:   columnDefs,
		HideRejected: hideRejected,
		HideAccepted: hideAccepted,
	}

	h.templates.RenderWithStatus(w, r, "pipeline.html", http.StatusOK, data, h.getLocale(r))
}
