package service

import (
	"context"
	"hr-sorter/internal/repository"
)

type FilterService struct {
	fltRepo *repository.FilterRepository
}

func NewFilterService(fltRepo *repository.FilterRepository) *FilterService {
	return &FilterService{
		fltRepo: fltRepo,
	}
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
