package hh

import (
	"context"
	"os"
)

// TokenResponse represents the JSON response from HeadHunter's token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// ClientConfig holds the HeadHunter OAuth credentials.
type ClientConfig struct {
	ClientID     string
	ClientSecret string
}

const (
	// Default values for HeadHunter Android app keys.
	// Can be overridden via environment variables.
	DefaultAndroidClientID     = "HIOMIAS39CA9DICTA7JIO64LQKQJF5AGIK74G9ITJKLNEDAOH5FHS5G1JI7FOEGD"
	DefaultAndroidClientSecret = "V9M870DE342BGHFRUJ5FTCGCUA1482AN0DI8C5TFI9ULMA89H10N60NOP8I4JMVS"
)

// GetDefaultClientConfig returns configuration from environment or defaults.
func GetDefaultClientConfig() ClientConfig {
	id := os.Getenv("HH_CLIENT_ID")
	if id == "" {
		id = DefaultAndroidClientID
	}
	secret := os.Getenv("HH_CLIENT_SECRET")
	if secret == "" {
		secret = DefaultAndroidClientSecret
	}
	return ClientConfig{
		ClientID:     id,
		ClientSecret: secret,
	}
}

// HHClient defines the high-level API for HeadHunter that automatically handles token refresh.
// It is used by the application to make authenticated requests.
type HHClient interface {
	// ExecuteRequest makes an authenticated request using the stored session.
	// It should automatically refresh the session if the token is expired (403 Forbidden).
	ExecuteRequest(ctx context.Context, session *Session, method string, path string, body interface{}) ([]byte, error)

	// RefreshToken exchanges the refresh_token for a new access_token.
	// Updates the session object in memory and storage upon success.
	RefreshToken(ctx context.Context, session *Session) error
}

// UserAgentGenerator creates a specific User-Agent following HH Android app spoofing rules.
// Example: ru.hh.android/7.20.1, Device: Pixel 7, Android OS: 13 (UUID: <uuid>)
type UserAgentGenerator interface {
	// Generate returns a randomized but valid HeadHunter Android User-Agent.
	Generate() string
}
