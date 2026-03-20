package tgclient

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"hr-sorter/internal/database"
	"hr-sorter/internal/models"
)

type Manager struct {
	clients map[int64]*telegram.Client
	mu      sync.RWMutex
	appID   int
	appHash string
}

func NewManager(appID int, appHash string) *Manager {
	return &Manager{
		clients: make(map[int64]*telegram.Client),
		appID:   appID,
		appHash: appHash,
	}
}

func (m *Manager) StartAccount(ctx context.Context, acc models.Account) error {
	if acc.SessionPath == "" {
		return fmt.Errorf("no session path for account %s", acc.PhoneNumber)
	}

	dispatcher := tg.NewUpdateDispatcher()
	client := telegram.NewClient(m.appID, m.appHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: acc.SessionPath,
		},
		UpdateHandler: dispatcher,
	})

	api := tg.NewClient(client)

	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		return m.HandleNewMessage(ctx, api, u, e.Users, acc.ID)
	})

	log.Printf("Starting account %s...", acc.PhoneNumber)

	return client.Run(ctx, func(ctx context.Context) error {
		log.Printf("Account %s is now running.", acc.PhoneNumber)
		<-ctx.Done()
		return ctx.Err()
	})
}

func (m *Manager) HandleNewMessage(ctx context.Context, api *tg.Client, u *tg.UpdateNewMessage, users map[int64]*tg.User, accountID int64) error {
	msg, ok := u.Message.(*tg.Message)
	if !ok {
		return nil
	}

	peer, ok := msg.PeerID.(*tg.PeerUser)
	if !ok {
		return nil
	}

	userID := peer.UserID
	text := msg.Message

	var user *tg.User
	user, ok = users[userID]
	if !ok {
		log.Printf("Fetching missing user info for %d...", userID)
		usersRes, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{
			UserID: userID,
		}})
		if err != nil {
			log.Printf("Failed to fetch user %d: %v", userID, err)
			return nil
		}

		if len(usersRes) > 0 {
			if u, ok := usersRes[0].(*tg.User); ok {
				user = u
			}
		}
	}

	if user == nil {
		log.Printf("Could not resolve user %d", userID)
		return nil
	}

	// Save contact if not exists
	_, err := database.DB.Exec("INSERT OR IGNORE INTO contacts (tg_user_id, first_name, last_name, username) VALUES (?, ?, ?, ?)",
		user.ID, user.FirstName, user.LastName, user.Username)
	if err != nil {
		log.Printf("DB Error saving contact: %v", err)
		return err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE tg_user_id = ?", user.ID)
	if err != nil {
		log.Printf("DB Error getting contact ID: %v", err)
		return err
	}

	// Save message
	_, err = database.DB.Exec("INSERT INTO messages (account_id, contact_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?)",
		accountID, contactID, text, !msg.Out, time.Unix(int64(msg.Date), 0))
	if err != nil {
		log.Printf("DB Error saving message: %v", err)
		return err
	}

	log.Printf("Successfully captured message from @%s (Internal ID: %d)", user.Username, contactID)
	return nil
}
