package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"strings"
	"time"
)

type SequenceService struct {
	seqRepo    *repository.SequenceRepository
	conRepo    *repository.ContactRepository
	accRepo    *repository.AccountRepository
	conService *ContactService
}

func NewSequenceService(seqRepo *repository.SequenceRepository, conRepo *repository.ContactRepository, accRepo *repository.AccountRepository, conService *ContactService) *SequenceService {
	return &SequenceService{
		seqRepo:    seqRepo,
		conRepo:    conRepo,
		accRepo:    accRepo,
		conService: conService,
	}
}

func (s *SequenceService) BulkCreateSequences(ctx context.Context, accountID, platform string, showDeclines, hideScreened, hideUnanswered bool) (int, error) {
	contacts, err := s.conService.GetFilteredContacts(ctx, accountID, platform, showDeclines, hideScreened, hideUnanswered, false, "")
	if err != nil {
		return 0, err
	}

	logger.Info(logger.AddSequence, "[BulkCreate] Starting for %d candidates", len(contacts))
	start := time.Now()

	// Start a single transaction for the entire bulk operation
	tx, err := s.seqRepo.BeginTx(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	count := 0

	for _, c := range contacts {
		if c.InSequence {
			continue
		}

		company := "Unknown"
		vacancy := "Direct Lead"

		if c.FirstName != nil && *c.FirstName != "" {
			company = *c.FirstName
		}

		if c.LastName != nil && *c.LastName != "" {
			vacancy = *c.LastName
		}

		// Internal logic similar to CreateSequence but using the existing TX
		seqID, err := s.seqRepo.Create(ctx, tx, c.AccountID, company, vacancy, "initial")
		if err != nil {
			logger.Error(logger.AddSequence, "[BulkCreate] Failed to create seq: %v", err)
			continue
		}

		if err := s.seqRepo.LinkContact(ctx, tx, seqID, c.ID); err != nil {
			logger.Error(logger.AddSequence, "[BulkCreate] Link error: %v", err)
		}

		initialDate := time.Now()
		stages := []models.InterviewStage{
			{SequenceID: seqID, Name: "Initial Contact", ScheduledAt: &initialDate, IsCompleted: true, OrderIndex: 0},
			{SequenceID: seqID, Name: "HR Screening", OrderIndex: 1},
			{SequenceID: seqID, Name: "Technical Interview", OrderIndex: 2},
			{SequenceID: seqID, Name: "Final Interview", OrderIndex: 3},
			{SequenceID: seqID, Name: "Offer", OrderIndex: 4},
		}

		if err := s.seqRepo.CreateStagesBatch(ctx, tx, stages); err != nil {
			logger.Error(logger.AddSequence, "[BulkCreate] Stages error: %v", err)
		}

		count++
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	logger.Info(logger.AddSequence, "[BulkCreate] SUCCESS: %d sequences created in %v", count, time.Since(start))
	return count, nil
}

func (s *SequenceService) CreateSequence(ctx context.Context, company, vacancy, contactID, initialDateStr string) (int64, error) {
	start := time.Now()
	logger.Trace(logger.AddSequence, "[CreateSequence] START for %s / %s (contact: %s)", company, vacancy, contactID)

	tx, err := s.seqRepo.BeginTx(ctx)
	if err != nil {
		logger.Error(logger.AddSequence, "[CreateSequence] Failed to begin tx: %v", err)
		return 0, err
	}
	defer tx.Rollback()

	logger.Trace(logger.AddSequence, "[CreateSequence] Transaction started after %v", time.Since(start))

	var accountID *int64
	if contactID != "" {
		accID, err := s.conRepo.GetAccountIDByContactID(ctx, contactID)
		if err == nil {
			accountID = accID
		}
	}

	if accountID == nil {
		activeAccounts, err := s.accRepo.GetActive(ctx)
		if err == nil && len(activeAccounts) > 0 {
			id := activeAccounts[0].ID
			accountID = &id
		} else {
			allAccounts, err := s.accRepo.GetAll(ctx)
			if err == nil && len(allAccounts) > 0 {
				id := allAccounts[0].ID
				accountID = &id
			}
		}
	}

	seqID, err := s.seqRepo.Create(ctx, tx, accountID, company, vacancy, "initial")
	if err != nil {
		logger.Error(logger.AddSequence, "[CreateSequence] Failed to create sequence row: %v", err)
		return 0, err
	}
	logger.Trace(logger.AddSequence, "[CreateSequence] Sequence created (ID: %d)", seqID)

	if contactID != "" {
		if err := s.seqRepo.LinkContact(ctx, tx, seqID, contactID); err != nil {
			logger.Error(logger.AddSequence, "[CreateSequence] Error linking contact: %v", err)
		} else {
			logger.Trace(logger.AddSequence, "[CreateSequence] Contact linked")
		}
	}

	initialDate := time.Now()
	if initialDateStr != "" {
		if d, err := time.Parse("2006-01-02T15:04", initialDateStr); err == nil {
			initialDate = d
		} else if d, err := time.Parse(time.RFC3339, initialDateStr); err == nil {
			initialDate = d
		}
	}

	stages := []models.InterviewStage{
		{SequenceID: seqID, Name: "Initial Contact", ScheduledAt: &initialDate, IsCompleted: true, OrderIndex: 0},
		{SequenceID: seqID, Name: "HR Screening", OrderIndex: 1},
		{SequenceID: seqID, Name: "Technical Interview", OrderIndex: 2},
		{SequenceID: seqID, Name: "Final Interview", OrderIndex: 3},
		{SequenceID: seqID, Name: "Offer", OrderIndex: 4},
	}

	if err := s.seqRepo.CreateStagesBatch(ctx, tx, stages); err != nil {
		logger.Error(logger.AddSequence, "[CreateSequence] Failed to create stages batch: %v", err)
		return 0, err
	}
	logger.Trace(logger.AddSequence, "[CreateSequence] Stages batch inserted")

	if err := tx.Commit(); err != nil {
		logger.Error(logger.AddSequence, "[CreateSequence] Transaction commit failed: %v", err)
		return 0, err
	}

	duration := time.Since(start)
	logger.Info(logger.AddSequence, "[CreateSequence] SUCCESS (ID: %d) in %v", seqID, duration)
	if duration > 500*time.Millisecond {
		logger.Warn(logger.AddSequence, "[PERF] CreateSequence took too long: %v", duration)
	}

	return seqID, nil
}

func (s *SequenceService) UpdateStageCompletion(ctx context.Context, stageID string, completed bool) error {
	stage, err := s.seqRepo.GetStageByID(ctx, stageID)
	if err != nil {
		return err
	}

	if !completed {
		standard := false
		lowerName := strings.ToLower(stage.Name)
		standards := []string{"initial contact", "hr screening", "technical interview", "final interview", "offer"}
		for _, std := range standards {
			if lowerName == std {
				standard = true
				break
			}
		}

		if !standard {
			if err := s.seqRepo.DeleteStage(ctx, stageID); err != nil {
				return err
			}
		} else {
			if err := s.seqRepo.UpdateStageStatus(ctx, stageID, false); err != nil {
				return err
			}
		}
	} else {
		if err := s.seqRepo.UpdateStageStatus(ctx, stageID, true); err != nil {
			return err
		}
	}

	// Auto status update
	last, err := s.seqRepo.GetLastCompletedStage(ctx, stage.SequenceID)
	if err == nil && last != nil {
		name := strings.ToLower(last.Name)
		newStatus := "initial"
		if strings.Contains(name, "offer") {
			newStatus = "offer"
		} else if strings.Contains(name, "final") {
			newStatus = "final"
		} else if strings.Contains(name, "tech") {
			newStatus = "tech"
		} else if strings.Contains(name, "screen") {
			newStatus = "screening"
		}
		return s.seqRepo.UpdateStatus(ctx, stage.SequenceID, newStatus)
	} else {
		return s.seqRepo.UpdateStatus(ctx, stage.SequenceID, "initial")
	}
}

func (s *SequenceService) MoveSequence(ctx context.Context, seqID int64, status string) error {
	if err := s.seqRepo.UpdateStatus(ctx, seqID, status); err != nil {
		return err
	}

	hierarchy := map[string]int{
		"initial":   0,
		"screening": 1,
		"tech":      2,
		"final":     3,
		"offer":     4,
		"accepted":  5,
		"rejected":  0,
	}

	targetRank, ok := hierarchy[status]
	if ok && status != "rejected" {
		stages, err := s.seqRepo.GetStages(ctx, seqID)
		if err == nil {
			for _, st := range stages {
				sName := strings.ToLower(st.Name)
				sRank := 0
				if strings.Contains(sName, "offer") {
					sRank = 4
				} else if strings.Contains(sName, "final") {
					sRank = 3
				} else if strings.Contains(sName, "tech") {
					sRank = 2
				} else if strings.Contains(sName, "screen") {
					sRank = 1
				}

				if sRank <= targetRank {
					s.seqRepo.UpdateStageStatus(ctx, st.ID, true)
				} else {
					s.seqRepo.UpdateStageStatus(ctx, st.ID, false)
				}
			}
		}
	}

	if status == "rejected" {
		incomplete, err := s.seqRepo.GetFirstIncompleteStage(ctx, seqID)
		if err == nil && incomplete != nil {
			s.seqRepo.UpdateStageStatus(ctx, incomplete.ID, true)
		}
	}

	return nil
}

func (s *SequenceService) AddStage(ctx context.Context, seqID int64, category, customName string) error {
	name := customName
	if name == "" {
		label := category
		switch category {
		case "tech":
			label = "Technical Interview"
		case "screening":
			label = "HR Screening"
		case "final":
			label = "Final Interview"
		case "offer":
			label = "Offer"
		}

		count, _ := s.seqRepo.GetStageCountByCategory(ctx, seqID, label, category)
		name = fmt.Sprintf("%s %d", strings.Title(label), count+1)
	}

	stages, _ := s.seqRepo.GetStages(ctx, seqID)
	hierarchy := map[string]int{
		"initial":   0,
		"screening": 1,
		"tech":      2,
		"final":     3,
		"offer":     4,
	}
	newRank := hierarchy[category]

	insertAt := 0
	if len(stages) > 0 {
		insertAt = stages[len(stages)-1].OrderIndex + 1
		for _, st := range stages {
			sName := strings.ToLower(st.Name)
			sRank := 0
			if strings.Contains(sName, "offer") {
				sRank = 4
			} else if strings.Contains(sName, "final") {
				sRank = 3
			} else if strings.Contains(sName, "tech") {
				sRank = 2
			} else if strings.Contains(sName, "screen") {
				sRank = 1
			}

			if sRank <= newRank {
				insertAt = st.OrderIndex + 1
			} else {
				insertAt = st.OrderIndex
				break
			}
		}
	}

	s.seqRepo.ShiftStages(ctx, seqID, insertAt)
	if err := s.seqRepo.CreateStage(ctx, nil, seqID, name, nil, true, insertAt); err != nil {
		return err
	}

	status := category
	if category == "initial" {
		status = "initial"
	}
	return s.seqRepo.UpdateStatus(ctx, seqID, status)
}
