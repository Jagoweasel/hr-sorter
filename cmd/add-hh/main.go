package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	HHApiURL      = "https://api.hh.ru/"
	HHOAuthURL    = "https://hh.ru/oauth/"
	DefaultConfig = "../hh-applicant-tool/config"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Token        Token  `json:"token"`
}

type Token struct {
	AccessToken     string `json:"access_token"`
	RefreshToken    string `json:"refresh_token"`
	AccessExpiresAt int    `json:"access_expires_at"`
}

func loadConfig(configDir, profile string) (*Config, error) {
	configPath := filepath.Join(configDir, profile, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func saveConfig(configDir, profile string, cfg *Config) error {
	configPath := filepath.Join(configDir, profile, "config.json")

	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func authorizeURL(clientID string) string {
	params := url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {"hhandroid://"},
		"response_type": {"code"},
	}
	return HHOAuthURL + "authorize?" + params.Encode()
}

func exchangeToken(clientID, clientSecret, code string) (*Token, error) {
	data := url.Values{
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
	}

	resp, err := http.Post(HHOAuthURL+"token", "application/x-www-form-urlencoded", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("request token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token exchange failed: status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if errMsg, ok := result["error"].(string); ok {
		return nil, fmt.Errorf("oauth error: %s", errMsg)
	}

	expiresIn, _ := result["expires_in"].(float64)

	return &Token{
		AccessToken:     result["access_token"].(string),
		RefreshToken:    result["refresh_token"].(string),
		AccessExpiresAt: int(time.Now().Unix()) + int(expiresIn),
	}, nil
}

func main() {
	configDir := flag.String("config-dir", DefaultConfig, "Config directory with profiles")
	profile := flag.String("profile", "egor", "Profile name (e.g. egor, george)")
	username := flag.String("username", "", "HH email or phone (optional, prompt if missing)")
	password := flag.String("password", "", "Password (optional)")
	flag.Parse()

	cfg, err := loadConfig(*configDir, *profile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		log.Fatal("client_id and client_secret must be set in config.json")
	}

	reader := bufio.NewReader(os.Stdin)

	usernameVal := *username
	if usernameVal == "" {
		fmt.Print("Enter email or phone: ")
		input, _ := reader.ReadString('\n')
		usernameVal = strings.TrimSpace(input)
		if usernameVal == "" {
			log.Fatal("username is required")
		}
	}

	fmt.Println("\nOAuth Flow:")
	fmt.Println("1. Go to:", authorizeURL(cfg.ClientID))
	fmt.Println("2. Login with username:", usernameVal)
	if *password != "" {
		fmt.Println("3. Password will be entered automatically (not implemented yet)")
	}
	fmt.Println("3. After login, you will be redirected to hhandroid://")
	fmt.Println("4. Extract the 'code' parameter from the redirect URL")
	fmt.Println("\nAlternatively, use --manual and enter the code after authorization")

	fmt.Print("\nEnter authorization code: ")
	code, _ := reader.ReadString('\n')
	code = strings.TrimSpace(code)

	if code == "" {
		log.Fatal("authorization code is required")
	}

	token, err := exchangeToken(cfg.ClientID, cfg.ClientSecret, code)
	if err != nil {
		log.Fatalf("Failed to exchange token: %v", err)
	}

	cfg.Token = *token
	if err := saveConfig(*configDir, *profile, cfg); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}

	fmt.Println("\n✓ Authorization successful!")
	fmt.Printf("Access token: %s...\n", token.AccessToken[:20])
	fmt.Printf("Expires at: %s\n", time.Unix(int64(token.AccessExpiresAt), 0))
	fmt.Printf("\nConfig saved to %s/%s/config.json\n", *configDir, *profile)
}
