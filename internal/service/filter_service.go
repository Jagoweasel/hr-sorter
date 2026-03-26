package service

import (
	"context"
	"encoding/json"
	"hr-sorter/internal/repository"
	"os"
)

type FilterService struct {
	fltRepo *repository.FilterRepository
}

func NewFilterService(fltRepo *repository.FilterRepository) *FilterService {
	return &FilterService{
		fltRepo: fltRepo,
	}
}

func (s *FilterService) ExportFilters(ctx context.Context) error {
	patterns, err := s.fltRepo.GetActivePatterns(ctx)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(patterns, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile("filters.json", data, 0644)
}

func (s *FilterService) AddFilter(ctx context.Context, pattern string) error {
	if pattern == "" {
		return nil
	}
	return s.fltRepo.Create(ctx, pattern)
}

func (s *FilterService) DeleteFilter(ctx context.Context, id string) error {
	return s.fltRepo.Delete(ctx, id)
}

func (s *FilterService) ToggleFilter(ctx context.Context, id string) error {
	return s.fltRepo.Toggle(ctx, id)
}
