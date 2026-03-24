package hhclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

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
		"redirect_uri":  {"hhandroid://"},
		"response_type": {"code"},
	}
	return HHOAuthURL + "authorize?" + params.Encode()
}

func ExchangeToken(code string) (*TokenResponse, error) {
	data := url.Values{
		"client_id":     {AndroidClientID},
		"client_secret": {AndroidClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.Post(HHOAuthURL+"token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: status %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	return &tr, nil
}

func RefreshToken(refreshToken string) (*TokenResponse, error) {
	data := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}

	resp, err := http.Post(HHOAuthURL+"token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token refresh failed: status %d", resp.StatusCode)
	}

	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return nil, err
	}

	return &tr, nil
}
