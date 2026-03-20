package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hr-sorter/internal/database"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "hr-sorter.db"
	}
	database.InitDB(dbPath)

	apiIDStr := os.Getenv("API_ID")
	apiHash := os.Getenv("API_HASH")

	if apiIDStr == "" || apiHash == "" {
		log.Fatal("API_ID or API_HASH not set in .env")
	}

	apiID, _ := strconv.Atoi(apiIDStr)

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter phone number (+123456789): ")
	phone, _ := reader.ReadString('\n')
	phone = strings.TrimSpace(phone)

	if phone == "" {
		log.Fatal("Phone number cannot be empty")
	}

	// Create sessions directory if it doesn't exist
	if err := os.MkdirAll("sessions", 0755); err != nil {
		log.Fatalf("failed to create sessions dir: %v", err)
	}

	sessionFile := filepath.Join("sessions", phone+".json")

	// Setup logger to see what's happening
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: sessionFile,
		},
		Logger: logger,
	})

	flow := auth.NewFlow(
		auth.Constant(phone, "", auth.CodeAuthenticatorFunc(func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
			fmt.Print("Enter code: ")
			code, _ := reader.ReadString('\n')
			return strings.TrimSpace(code), nil
		})),
		auth.SendCodeOptions{},
	)

	fmt.Println("Connecting to Telegram...")

	if err := client.Run(context.Background(), func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}

		// Save account to DB
		_, err := database.DB.Exec("INSERT OR IGNORE INTO accounts (phone_number, status, session_path) VALUES (?, ?, ?)",
			phone, "active", sessionFile)
		if err != nil {
			return err
		}

		fmt.Println("\nSuccess! Account added.")
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
