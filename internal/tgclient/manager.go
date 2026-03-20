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

	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		return m.HandleNewMessage(ctx, u, e.Users, acc.ID)
	})

	log.Printf("Starting account %s...", acc.PhoneNumber)

	return client.Run(ctx, func(ctx context.Context) error {
		// Just wait for context cancellation
		log.Printf("Account %s is now running.", acc.PhoneNumber)
		<-ctx.Done()
		return ctx.Err()
	})
}

func (m *Manager) HandleNewMessage(ctx context.Context, u *tg.UpdateNewMessage, users map[int64]*tg.User, accountID int64) error {
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

	user, ok := users[userID]
	if !ok {
		// If we don't have user in current entities, we might skip it or fetch it.
		// For now, let's at least log the ID.
		log.Printf("Message from unknown user %d", userID)
		return nil
	}

	// Save contact if not exists
	_, err := database.DB.Exec("INSERT OR IGNORE INTO contacts (tg_user_id, first_name, last_name, username) VALUES (?, ?, ?, ?)",
		user.ID, user.FirstName, user.LastName, user.Username)
	if err != nil {
		return err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE tg_user_id = ?", user.ID)
	if err != nil {
		return err
	}

	// Save message
	_, err = database.DB.Exec("INSERT INTO messages (account_id, contact_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?)",
		accountID, contactID, text, !msg.Out, time.Unix(int64(msg.Date), 0))

	log.Printf("Captured message from @%s on account %d", user.Username, accountID)
	return err
}
