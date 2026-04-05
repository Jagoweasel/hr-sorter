package hhclient

import (
	"context"
	"hr-sorter/internal/domain/dto"
	"hr-sorter/internal/models"
	"testing"
)

type MockRepo struct {
}

func (m *MockRepo) SaveAccount(ctx context.Context, account *models.Account) error { return nil }
func (m *MockRepo) GetAccount(ctx context.Context, id int64) (*models.Account, error) {
	return nil, nil
}
func (m *MockRepo) SaveNegotiations(ctx context.Context, negotiations []dto.HHNegotiation) error {
	return nil
}
func (m *MockRepo) SaveNegotiationsStats(ctx context.Context, stats *models.NegotiationStats) error {
	return nil
}
func (m *MockRepo) GetMappingRules(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (m *MockRepo) GetMessageFilters(ctx context.Context) ([]models.MessageFilter, error) {
	return nil, nil
}
func (m *MockRepo) SaveMessageFilter(ctx context.Context, filter *models.MessageFilter) error {
	return nil
}
func (m *MockRepo) GetIntegration(ctx context.Context, id int64) (*models.Integration, error) {
	return nil, nil
}
func (m *MockRepo) SaveIntegration(ctx context.Context, integration *models.Integration) error {
	return nil
}
func (m *MockRepo) GetSequence(ctx context.Context, id int64) (*models.Sequence, error) {
	return nil, nil
}
func (m *MockRepo) SaveSequence(ctx context.Context, sequence *models.Sequence) error {
	return nil
}

func TestHHAuthService_StateTransitions(t *testing.T) {
	repo := &MockRepo{}
	s := NewHHAuthService(repo)
	ctx := context.Background()

	t.Run("InitialState", func(t *testing.T) {
		status, _ := s.GetStatus(ctx)
		if status.State != dto.AuthStateNone {
			t.Errorf("expected %s, got %s", dto.AuthStateNone, status.State)
		}
	})

	t.Run("StartAuthTransition", func(t *testing.T) {
		// Note: this will start a goroutine that might fail due to no playwright browser,
		// but we just check the initial transition.
		status, err := s.StartAuth(ctx, "test@example.com")
		if err != nil {
			t.Fatalf("failed to start auth: %v", err)
		}
		if status.State != dto.AuthStateWaitIdentify {
			t.Errorf("expected %s, got %s", dto.AuthStateWaitIdentify, status.State)
		}
	})
}
