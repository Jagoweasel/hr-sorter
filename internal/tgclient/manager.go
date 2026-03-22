package tgclient

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
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
	log.Printf("[Account %s] [STARTING] (ID: %d, Path: %s)", acc.PhoneNumber, acc.ID, acc.SessionPath)
	if acc.SessionPath == "" {
		return fmt.Errorf("no session path for account %s", acc.PhoneNumber)
	}

	dispatcher := tg.NewUpdateDispatcher()

	// Ensure absolute path and use forward slashes for cross-platform compatibility
	sessionPath, _ := filepath.Abs(acc.SessionPath)
	sessionPath = filepath.ToSlash(sessionPath)

	client := telegram.NewClient(m.appID, m.appHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: sessionPath,
		},
		UpdateHandler: dispatcher,
	})

	api := tg.NewClient(client)

	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		return m.HandleNewMessage(ctx, api, u, e.Users, acc.ID)
	})

	log.Printf("[Account %s] [INIT] Client setup complete, starting run...", acc.PhoneNumber)

	return client.Run(ctx, func(ctx context.Context) error {
		log.Printf("[Account %s] [RUNNING] Logged in successfully.", acc.PhoneNumber)

		// Trigger initial sync in background
		go func() {
			log.Printf("[Account %s] [SYNC] Triggering background initial sync in 2 seconds...", acc.PhoneNumber)
			time.Sleep(2 * time.Second) // Small delay to ensure client is fully ready
			if err := m.InitialSync(ctx, api, acc.ID); err != nil {
				log.Printf("[Account %s] [ERROR] Initial sync failed: %v", acc.PhoneNumber, err)
			}
		}()

		<-ctx.Done()
		log.Printf("[Account %s] [STOP] Shutdown signal received.", acc.PhoneNumber)
		return ctx.Err()
	})
}

func (m *Manager) getOrCreateContact(ctx context.Context, user *tg.User) (int64, error) {
	_, err := database.DB.Exec("INSERT OR IGNORE INTO contacts (tg_user_id, first_name, last_name, username) VALUES (?, ?, ?, ?)",
		user.ID, user.FirstName, user.LastName, user.Username)
	if err != nil {
		return 0, err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE tg_user_id = ?", user.ID)
	return contactID, err
}

func (m *Manager) InitialSync(ctx context.Context, api *tg.Client, accountID int64) error {
	// Verify session first
	me, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		return fmt.Errorf("failed to verify session: %w", err)
	}
	if len(me) > 0 {
		if u, ok := me[0].(*tg.User); ok {
			log.Printf("[Acc ID %d] Session verified as @%s (%s %s)", accountID, u.Username, u.FirstName, u.LastName)
		}
	}

	// Sync both main inbox (Folder 0) and archive (Folder 1)
	folders := []int{0, 1}
	for _, folderID := range folders {
		folderName := "Inbox"
		if folderID == 1 {
			folderName = "Archive"
		}
		log.Printf("[Acc ID %d] Scanning %s (fetching up to 500 dialogs)...", accountID, folderName)

		res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      500,
			FolderID:   folderID,
		})
		if err != nil {
			log.Printf("[Acc ID %d] Failed to fetch %s dialogs: %v", accountID, folderName, err)
			continue
		}

		var users map[int64]*tg.User = make(map[int64]*tg.User)
		var dialogs []tg.DialogClass

		switch d := res.(type) {
		case *tg.MessagesDialogs:
			dialogs = d.Dialogs
			for _, uClass := range d.Users {
				if u, ok := uClass.(*tg.User); ok {
					users[u.ID] = u
				}
			}
		case *tg.MessagesDialogsSlice:
			dialogs = d.Dialogs
			for _, uClass := range d.Users {
				if u, ok := uClass.(*tg.User); ok {
					users[u.ID] = u
				}
			}
		}

		log.Printf("[Acc ID %d] Found %d total dialogs in %s", accountID, len(dialogs), folderName)

		if len(dialogs) == 0 {
			log.Printf("[Acc ID %d] WARNING: No dialogs returned for folder %s", accountID, folderName)
		}

		for _, dClass := range dialogs {
			d, ok := dClass.(*tg.Dialog)
			if !ok {
				continue
			}

			// Trace log for discovery
			peerType := "Unknown"
			peerName := "Unknown"
			var targetUser *tg.User

			switch p := d.Peer.(type) {
			case *tg.PeerUser:
				peerType = "User"
				if u, ok := users[p.UserID]; ok {
					targetUser = u
					peerName = fmt.Sprintf("@%s (%s %s)", u.Username, u.FirstName, u.LastName)
				} else {
					peerName = fmt.Sprintf("ID:%d", p.UserID)
				}
			case *tg.PeerChat:
				peerType = "Chat"
				peerName = fmt.Sprintf("ID:%d", p.ChatID)
			case *tg.PeerChannel:
				peerType = "Channel"
				peerName = fmt.Sprintf("ID:%d", p.ChannelID)
			}

			if targetUser != nil {
				// It's a private chat
				if targetUser.Bot {
					log.Printf("[Trace] Skipping bot: %s", peerName)
					continue
				}

				log.Printf("[Trace] Syncing private chat: %s", peerName)
				contactID, err := m.getOrCreateContact(ctx, targetUser)
				if err != nil {
					log.Printf("[Acc ID %d] Failed to ensure contact %s: %v", accountID, peerName, err)
					continue
				}

				if err := m.SyncHistory(ctx, api, targetUser, contactID, accountID); err != nil {
					log.Printf("[Acc ID %d] Failed to sync history for %s: %v", accountID, peerName, err)
				}
			} else {
				log.Printf("[Trace] Skipping %s: %s", peerType, peerName)
			}
		}
	}

	log.Printf("[Acc ID %d] Initial sync finished", accountID)
	return nil
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
		log.Printf("[Acc ID %d] Fetching missing user info for %d...", accountID, userID)
		usersRes, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{
			UserID: userID,
		}})
		if err != nil {
			log.Printf("[Acc ID %d] Failed to fetch user %d: %v", accountID, userID, err)
			return nil
		}

		if len(usersRes) > 0 {
			if u, ok := usersRes[0].(*tg.User); ok {
				user = u
			}
		}
	}

	if user == nil {
		log.Printf("[Acc ID %d] Could not resolve user %d", accountID, userID)
		return nil
	}

	contactID, err := m.getOrCreateContact(ctx, user)
	if err != nil {
		log.Printf("[Acc ID %d] DB Error handling contact: %v", accountID, err)
		return err
	}

	// Save message
	_, err = database.DB.Exec("INSERT OR IGNORE INTO messages (account_id, contact_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?)",
		accountID, contactID, text, !msg.Out, time.Unix(int64(msg.Date), 0))
	if err != nil {
		log.Printf("[Acc ID %d] DB Error saving message: %v", accountID, err)
		return err
	}

	log.Printf("[Acc ID %d] Captured message from @%s", accountID, user.Username)

	// Background sync history for this user to ensure we didn't miss anything
	go func() {
		if err := m.SyncHistory(ctx, api, user, contactID, accountID); err != nil {
			log.Printf("[Acc ID %d] Background sync failed for @%s: %v", accountID, user.Username, err)
		}
	}()

	return nil
}

func (m *Manager) SyncHistory(ctx context.Context, api *tg.Client, user *tg.User, contactID, accountID int64) error {
	log.Printf("[Acc ID %d] Syncing history for @%s...", accountID, user.Username)

	// Telegram might return different types of message lists
	var messages []tg.MessageClass

	history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash},
		Limit: 100,
	})
	if err != nil {
		return err
	}

	switch h := history.(type) {
	case *tg.MessagesMessages:
		messages = h.Messages
	case *tg.MessagesMessagesSlice:
		messages = h.Messages
	case *tg.MessagesChannelMessages:
		messages = h.Messages
	}

	newMsgs := 0
	for _, mClass := range messages {
		msg, ok := mClass.(*tg.Message)
		if !ok {
			continue
		}

		// Use INSERT OR IGNORE to avoid duplicates if message was already captured live
		res, err := database.DB.Exec("INSERT OR IGNORE INTO messages (account_id, contact_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?)",
			accountID, contactID, msg.Message, !msg.Out, time.Unix(int64(msg.Date), 0))
		if err != nil {
			log.Printf("[Acc ID %d] DB Error during sync for @%s: %v", accountID, user.Username, err)
			continue
		}

		rows, _ := res.RowsAffected()
		if rows > 0 {
			newMsgs++
		}
	}

	if newMsgs > 0 {
		log.Printf("[Acc ID %d] Finished sync for @%s: saved %d new messages", accountID, user.Username, newMsgs)
	}
	return nil
}
