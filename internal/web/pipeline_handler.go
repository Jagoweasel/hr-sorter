package web

import (
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"net/http"
	"strconv"
)

type AccountGroup struct {
	Account   *models.Account
	Columns   []PipelineColumn
	Sequences []repository.SequenceWithDetails
}

type PipelineColumn struct {
	models.ColumnDef
	Sequences []repository.SequenceWithDetails
}

func (h *Handler) handlePipeline(w http.ResponseWriter, r *http.Request) {
	view := r.FormValue("view")
	if view == "" {
		view = h.getCookie(r, "pipe_view", "kanban")
	}

	activeAccountID := r.FormValue("account_id")
	if activeAccountID == "" {
		activeAccountID = h.getCookie(r, "filter_account_id", "")
	}

	logger.Trace(logger.AddSequence, "[Web] handlePipeline: View=%s, AccountID=%s", view, activeAccountID)

	hideRejectedStr := r.FormValue("hide_rejected")
	if hideRejectedStr == "" {
		hideRejectedStr = h.getCookie(r, "pipe_hide_rejected", "false")
	}
	hideRejected := hideRejectedStr == "true"

	hideAcceptedStr := r.FormValue("hide_accepted")
	if hideAcceptedStr == "" {
		hideAcceptedStr = h.getCookie(r, "pipe_hide_accepted", "false")
	}
	hideAccepted := hideAcceptedStr == "true"

	allAccounts, _ := h.accRepo.GetAll(r.Context())
	sequences, err := h.seqRepo.GetAll(r.Context(), activeAccountID)
	if err != nil {
		logger.Error(logger.AddSequence, "[Web] handlePipeline: Error fetching sequences: %v", err)
		http.Error(w, err.Error(), 500)
		return
	}

	logger.Trace(logger.AddSequence, "[Web] handlePipeline: Loaded %d raw sequences", len(sequences))

	var seqIDs []int64
	for _, s := range sequences {
		if hideRejected && s.Status == "rejected" {
			continue
		}
		if hideAccepted && s.Status == "accepted" {
			continue
		}
		seqIDs = append(seqIDs, s.ID)
	}

	recruitersMap, _ := h.seqRepo.GetRecruitersBatch(r.Context(), seqIDs)
	stagesMap, _ := h.seqRepo.GetStagesBatch(r.Context(), seqIDs)

	columnDefs := h.getColumnDefs(r)

	var detailedSeqs []repository.SequenceWithDetails
	for _, s := range sequences {
		if hideRejected && s.Status == "rejected" {
			continue
		}
		if hideAccepted && s.Status == "accepted" {
			continue
		}

		recruiters := recruitersMap[s.ID]
		stages := stagesMap[s.ID]

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
			ColumnDefs: columnDefs, // Set it here!
			IsRejected: s.Status == "rejected",
			IsAccepted: s.Status == "accepted",
		})
		logger.LogChain(s.ID, s.CompanyName, historyNames, s.Status)
	}

	var groups []*AccountGroup
	orphanGroup := &AccountGroup{Account: nil}
	for _, def := range columnDefs {
		orphanGroup.Columns = append(orphanGroup.Columns, PipelineColumn{ColumnDef: def})
	}

	accountMap := make(map[int64]*AccountGroup)
	for i := range allAccounts {
		acc := allAccounts[i]

		// If filtering by account, skip others
		if activeAccountID != "" && strconv.FormatInt(acc.ID, 10) != activeAccountID {
			continue
		}

		group := &AccountGroup{Account: &acc}
		for _, def := range columnDefs {
			group.Columns = append(group.Columns, PipelineColumn{ColumnDef: def})
		}
		groups = append(groups, group)
		accountMap[acc.ID] = group
	}

	for i := range detailedSeqs {
		s := &detailedSeqs[i]
		var targetGroup *AccountGroup
		if s.AccountID != nil {
			targetGroup = accountMap[*s.AccountID]
		}
		if targetGroup == nil {
			targetGroup = orphanGroup
		}

		targetGroup.Sequences = append(targetGroup.Sequences, *s)
		for j := range targetGroup.Columns {
			if s.Status == targetGroup.Columns[j].ID {
				targetGroup.Columns[j].Sequences = append(targetGroup.Columns[j].Sequences, *s)
			}
		}
	}

	if len(orphanGroup.Sequences) > 0 {
		groups = append(groups, orphanGroup)
	}

	data := struct {
		View            string
		Groups          []*AccountGroup
		ColumnDefs      []models.ColumnDef
		Accounts        []models.Account
		ActiveAccountID string
		HideRejected    bool
		HideAccepted    bool
	}{
		View:            view,
		Groups:          groups,
		ColumnDefs:      columnDefs,
		Accounts:        allAccounts,
		ActiveAccountID: activeAccountID,
		HideRejected:    hideRejected,
		HideAccepted:    hideAccepted,
	}

	h.templates.RenderWithStatus(w, r, "pipeline.html", http.StatusOK, data, h.getLocale(r))
}
