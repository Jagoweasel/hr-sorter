package hhclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hr-sorter/internal/logger"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type loggingRoundTripper struct {
	next http.RoundTripper
}

func (l *loggingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	logger.Debug(logger.HH, "--> %s %s", req.Method, req.URL.String())
	for k, v := range req.Header {
		if k == "Authorization" {
			logger.Debug(logger.HH, "Header: %s: Bearer [REDACTED]", k)
		} else {
			logger.Debug(logger.HH, "Header: %s: %v", k, v)
		}
	}

	resp, err := l.next.RoundTrip(req)
	if err != nil {
		logger.Debug(logger.HH, "<-- ERROR: %v", err)
		return nil, err
	}

	logger.Debug(logger.HH, "<-- %d %s", resp.StatusCode, req.URL.String())

	// Log response body for non-binary content
	if !strings.Contains(resp.Header.Get("Content-Type"), "image") {
		body, _ := io.ReadAll(resp.Body)
		resp.Body = io.NopCloser(bytes.NewBuffer(body))
		if len(body) > 0 {
			logger.Debug(logger.HH, "Response Body: %s", string(body))
		}
	}

	return resp, nil
}

func GetHHHttpClient() *http.Client {
	if logger.IsEnabled(logger.HH) {
		return &http.Client{
			Transport: &loggingRoundTripper{next: http.DefaultTransport},
		}
	}
	return http.DefaultClient
}

const (
	HHApiURL   = "https://api.hh.ru/"
	HHOAuthURL = "https://hh.ru/oauth/"

	// From hh-applicant-tool
	AndroidClientID     = "HIOMIAS39CA9DICTA7JIO64LQKQJF5AGIK74G9ITJKLNEDAOH5FHS5G1JI7FOEGD"
	AndroidClientSecret = "V9M870DE342BGHFRUJ5FTCGCUA1482AN0DI8C5TFI9ULMA89H10N60NOP8I4JMVS"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

func GetAuthorizeURL() string {
	params := url.Values{
		"client_id":     {AndroidClientID},
		"response_type": {"code"},
	}
	return HHOAuthURL + "authorize?" + params.Encode()
}

func ExchangeToken(code string, userAgent string) (*TokenResponse, error) {
	logger.Debug(logger.HH, "Exchanging code for token...")
	data := url.Values{
		"client_id":     {AndroidClientID},
		"client_secret": {AndroidClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	req, _ := http.NewRequest("POST", HHOAuthURL+"token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-HH-App-Active", "true")

	client := GetHHHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug(logger.HH, "Token exchange request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: status %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		logger.Debug(logger.HH, "Failed to decode token response: %v", err)
		return nil, err
	}

	logger.Debug(logger.HH, "Token exchange successful!")
	return &tr, nil
}

func RefreshToken(refreshToken string, userAgent string) (*TokenResponse, error) {
	logger.Debug(logger.HH, "Refreshing access token...")
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	req, _ := http.NewRequest("POST", HHOAuthURL+"token", strings.NewReader(data.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-HH-App-Active", "true")

	client := GetHHHttpClient()
	resp, err := client.Do(req)
	if err != nil {
		logger.Debug(logger.HH, "Token refresh request failed: %v", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed: status %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		logger.Debug(logger.HH, "Failed to decode refresh response: %v", err)
		return nil, err
	}

	logger.Debug(logger.HH, "Token refresh successful!")
	return &tr, nil
}
