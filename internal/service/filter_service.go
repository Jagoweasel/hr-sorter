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

	f, err := os.Create("filters.json")
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Write(data); err != nil {
		return err
	}

	return f.Sync()
}

func (s *FilterService) ImportFilters(ctx context.Context) error {
	path := "filters.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	return s.ImportFromJSON(ctx, data)
}

func (s *FilterService) ImportFromJSON(ctx context.Context, data []byte) error {
	var patterns []string
	if err := json.Unmarshal(data, &patterns); err != nil {
		return err
	}

	for _, p := range patterns {
		if p == "" {
			continue
		}
		_ = s.fltRepo.Create(ctx, p)
	}

	return nil
}

func (s *FilterService) ClearFilters(ctx context.Context) error {
	return s.fltRepo.DeleteAll(ctx)
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
