package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"hr-sorter/internal/database"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	database.InitDB(os.Getenv("DB_PATH"))

	apiID, _ := strconv.Atoi(os.Getenv("API_ID"))
	apiHash := os.Getenv("API_HASH")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter phone number (+123456789): ")
	phone, _ := reader.ReadString('\n')
	phone = strings.TrimSpace(phone)

	client := telegram.NewClient(apiID, apiHash, telegram.Options{})

	fmt.Println("Created client")

	flow := auth.NewFlow(
		auth.Constant(phone, "", auth.CodeAuthenticatorFunc(func(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
			fmt.Print("Enter code: ")
			code, _ := reader.ReadString('\n')
			return strings.TrimSpace(code), nil
		})),
		auth.SendCodeOptions{},
	)

	fmt.Println("Started flow")

	if err := client.Run(context.Background(), func(ctx context.Context) error {
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}

		// Save account to DB
		_, err := database.DB.Exec("INSERT OR IGNORE INTO accounts (phone_number, status) VALUES (?, ?)", phone, "active")
		if err != nil {
			return err
		}

		fmt.Println("Success! Account added.")
		return nil
	}); err != nil {
		log.Fatal(err)
	}
}
