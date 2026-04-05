package hh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockSessionStorage struct {
	mock.Mock
}

func (m *MockSessionStorage) Save(ctx context.Context, session *Session) error {
	args := m.Called(ctx, session)
	return args.Error(0)
}

func (m *MockSessionStorage) Get(ctx context.Context, accountID int64) (*Session, error) {
	args := m.Called(ctx, accountID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*Session), args.Error(1)
}

func (m *MockSessionStorage) Delete(ctx context.Context, accountID int64) error {
	args := m.Called(ctx, accountID)
	return args.Error(0)
}

func (m *MockSessionStorage) ListAll(ctx context.Context) ([]*Session, error) {
	args := m.Called(ctx)
	return args.Get(0).([]*Session), args.Error(1)
}

func TestHHClientWrapper_ExecuteRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	storage := new(MockSessionStorage)
	client := &HHClientWrapper{
		httpClient: server.Client(),
		storage:    storage,
		config:     ClientConfig{ClientID: "id", ClientSecret: "secret"},
		apiBaseURL: server.URL,
	}

	session := &Session{AccessToken: "access-token"}
	resp, err := client.ExecuteRequest(context.Background(), session, "GET", "/test", nil)
	require.NoError(t, err)

	var data map[string]string
	_ = json.Unmarshal(resp, &data)
	assert.Equal(t, "ok", data["status"])
}

func TestHHClientWrapper_ExecuteRequest_Refresh(t *testing.T) {
	refreshCalled := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			refreshCalled++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600}`))
			return
		}

		if r.Header.Get("Authorization") == "Bearer old-access" {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		assert.Equal(t, "Bearer new-access", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	storage := new(MockSessionStorage)
	storage.On("Save", mock.Anything, mock.Anything).Return(nil)

	client := &HHClientWrapper{
		httpClient: server.Client(),
		storage:    storage,
		config:     ClientConfig{ClientID: "id", ClientSecret: "secret"},
		apiBaseURL: server.URL,
		authURL:    server.URL + "/oauth/token",
	}

	session := &Session{
		AccountID:    1,
		AccessToken:  "old-access",
		RefreshToken: "old-refresh",
		ExpiresAt:    time.Now().Add(-time.Hour), // Expired
		UserAgent:    "old-ua",
	}

	resp, err := client.ExecuteRequest(context.Background(), session, "GET", "/test", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, refreshCalled)

	var data map[string]string
	_ = json.Unmarshal(resp, &data)
	assert.Equal(t, "ok", data["status"])

	access, refresh, _ := session.GetTokens()
	assert.Equal(t, "new-access", access)
	assert.Equal(t, "new-refresh", refresh)
	storage.AssertCalled(t, "Save", mock.Anything, session)
}

func TestHHClientWrapper_ExecuteRequest_ConcurrentRefresh(t *testing.T) {
	refreshCalled := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			time.Sleep(100 * time.Millisecond)
			refreshCalled++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer server.Close()

	storage := new(MockSessionStorage)
	storage.On("Save", mock.Anything, mock.Anything).Return(nil)

	client := &HHClientWrapper{
		httpClient: server.Client(),
		storage:    storage,
		config:     ClientConfig{ClientID: "id", ClientSecret: "secret"},
		apiBaseURL: server.URL,
		authURL:    server.URL + "/oauth/token",
	}

	session := &Session{
		AccountID:    1,
		RefreshToken: "refresh",
		ExpiresAt:    time.Now().Add(-time.Hour),
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.ExecuteRequest(context.Background(), session, "GET", "/test", nil)
		}()
	}
	wg.Wait()

	assert.Equal(t, 1, refreshCalled)
}
