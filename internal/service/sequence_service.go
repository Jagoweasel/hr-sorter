package service

import (
	"context"
	"fmt"
	"hr-sorter/internal/logger"
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

	count := 0
	now := time.Now().Format("2006-01-02T15:04")

	for _, c := range contacts {
		if c.InSequence {
			continue
		}

		company := ""
		vacancy := "Senior Go Dev"
		if c.Platform == "hh" {
			if c.FirstName != nil {
				company = *c.FirstName
			}
			if c.LastName != nil {
				vacancy = *c.LastName
			}
		}

		_, err := s.CreateSequence(ctx, company, vacancy, fmt.Sprintf("%d", c.ID), now)
		if err == nil {
			count++
		}
	}

	return count, nil
}

func (s *SequenceService) CreateSequence(ctx context.Context, company, vacancy, contactID, initialDateStr string) (int64, error) {
	tx, err := s.seqRepo.BeginTx(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

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
		return 0, err
	}

	if contactID != "" {
		if err := s.seqRepo.LinkContact(ctx, tx, seqID, contactID); err != nil {
			logger.Debug(logger.AddSequence, "Error linking contact: %v", err)
		}
	}

	initialDate, _ := time.Parse("2006-01-02T15:04", initialDateStr)
	s.seqRepo.CreateStage(ctx, tx, seqID, "Initial Contact", initialDate, true, 0)
	s.seqRepo.CreateStage(ctx, tx, seqID, "HR Screening", nil, false, 1)
	s.seqRepo.CreateStage(ctx, tx, seqID, "Technical Interview", nil, false, 2)
	s.seqRepo.CreateStage(ctx, tx, seqID, "Final Interview", nil, false, 3)
	s.seqRepo.CreateStage(ctx, tx, seqID, "Offer", nil, false, 4)

	if err := tx.Commit(); err != nil {
		return 0, err
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
