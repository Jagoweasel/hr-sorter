package tgclient

import (
	"context"
	"log"
	"sync"
	"time"

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

func (m *Manager) HandleUpdate(ctx context.Context, u tg.UpdatesClass, accountID int64) error {
	switch update := u.(type) {
	case *tg.Updates:
		for _, upd := range update.Updates {
			switch u := upd.(type) {
			case *tg.UpdateNewMessage:
				return m.HandleNewMessage(ctx, u, update.Users, accountID)
			}
		}
	}
	return nil
}

func (m *Manager) HandleNewMessage(ctx context.Context, u *tg.UpdateNewMessage, users []tg.UserClass, accountID int64) error {
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

	var contact models.Contact
	for _, userClass := range users {
		user, ok := userClass.(*tg.User)
		if ok && user.ID == userID {
			contact.TGUserID = user.ID
			contact.FirstName = user.FirstName
			contact.LastName = user.LastName
			contact.Username = user.Username
			break
		}
	}

	// Save contact if not exists
	res, err := database.DB.Exec("INSERT OR IGNORE INTO contacts (tg_user_id, first_name, last_name, username) VALUES (?, ?, ?, ?)",
		contact.TGUserID, contact.FirstName, contact.LastName, contact.Username)
	if err != nil {
		return err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE tg_user_id = ?", contact.TGUserID)
	if err != nil {
		// If IGNORE was triggered, we still need the ID
		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE tg_user_id = ?", contact.TGUserID)
		}
	}

	// Save message
	_, err = database.DB.Exec("INSERT INTO messages (account_id, contact_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?)",
		accountID, contactID, text, !msg.Out, time.Unix(int64(msg.Date), 0))

	log.Printf("Captured message from contact %d", contactID)
	return err
}
