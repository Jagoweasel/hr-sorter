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
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
)

type Manager struct {
	clients map[int64]*telegram.Client
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[int64]*telegram.Client),
	}
}

func (m *Manager) StartIntegration(ctx context.Context, integration models.Integration) error {
	log.Printf("[Integration %s] [STARTING] (ID: %d, Path: %s)", integration.Identifier, integration.ID, integration.SessionPath)
	if integration.SessionPath == "" {
		return fmt.Errorf("no session path for integration %s", integration.Identifier)
	}

	if integration.APIID == 0 || integration.APIHash == "" {
		return fmt.Errorf("missing API credentials for integration %s", integration.Identifier)
	}

	dispatcher := tg.NewUpdateDispatcher()

	// Ensure absolute path and use forward slashes for cross-platform compatibility
	sessionPath, _ := filepath.Abs(integration.SessionPath)
	sessionPath = filepath.ToSlash(sessionPath)

	client := telegram.NewClient(integration.APIID, integration.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: sessionPath,
		},
		UpdateHandler: dispatcher,
	})

	api := tg.NewClient(client)

	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		return m.HandleNewMessage(ctx, api, u, e.Users, integration.ID)
	})

	log.Printf("[Integration %s] [INIT] Client setup complete, starting run...", integration.Identifier)

	return client.Run(ctx, func(ctx context.Context) error {
		log.Printf("[Integration %s] [RUNNING] Logged in successfully.", integration.Identifier)

		// Trigger initial sync in background
		go func() {
			logger.Debug(logger.Sync, "[Integration %s] Triggering background initial sync in 2 seconds...", integration.Identifier)
			time.Sleep(2 * time.Second) // Small delay to ensure client is fully ready
			if err := m.InitialSync(ctx, api, integration.ID); err != nil {
				log.Printf("[Integration %s] [ERROR] Initial sync failed: %v", integration.Identifier, err)
			}
		}()

		<-ctx.Done()
		log.Printf("[Integration %s] [STOP] Shutdown signal received.", integration.Identifier)
		return ctx.Err()
	})
}

func (m *Manager) getOrCreateContact(ctx context.Context, user *tg.User, integrationID int64) (int64, error) {
	externalID := fmt.Sprintf("%d", user.ID)
	_, err := database.DB.Exec("INSERT OR IGNORE INTO contacts (integration_id, platform, external_id, first_name, last_name, username) VALUES (?, 'tg', ?, ?, ?, ?)",
		integrationID, externalID, user.FirstName, user.LastName, user.Username)
	if err != nil {
		return 0, err
	}

	var contactID int64
	err = database.DB.Get(&contactID, "SELECT id FROM contacts WHERE platform = 'tg' AND external_id = ?", externalID)
	return contactID, err
}

func (m *Manager) InitialSync(ctx context.Context, api *tg.Client, integrationID int64) error {
	// Verify session first
	me, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		return fmt.Errorf("failed to verify session: %w", err)
	}
	if len(me) > 0 {
		if u, ok := me[0].(*tg.User); ok {
			logger.Debug(logger.Sync, "[Int ID %d] Session verified as @%s (%s %s)", integrationID, u.Username, u.FirstName, u.LastName)
		}
	}

	// Map to track peers across folders to avoid duplicate syncs
	type peerInfo struct {
		user     *tg.User
		name     string
		peerType string
	}
	uniquePeers := make(map[int64]peerInfo)

	// Sync both main inbox (Folder 0) and archive (Folder 1)
	folders := []int{0, 1}
	for _, folderID := range folders {
		folderName := "Inbox"
		if folderID == 1 {
			folderName = "Archive"
		}
		logger.Debug(logger.Sync, "[Int ID %d] Scanning %s (fetching up to 500 dialogs)...", integrationID, folderName)

		res, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      500,
			FolderID:   folderID,
		})
		if err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to fetch %s dialogs: %v", integrationID, folderName, err)
			continue
		}

		var users = make(map[int64]*tg.User)
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

		logger.Debug(logger.Sync, "[Int ID %d] Found %d total dialogs in %s", integrationID, len(dialogs), folderName)

		for _, dClass := range dialogs {
			d, ok := dClass.(*tg.Dialog)
			if !ok {
				continue
			}

			switch p := d.Peer.(type) {
			case *tg.PeerUser:
				if u, ok := users[p.UserID]; ok {
					if _, exists := uniquePeers[u.ID]; !exists {
						uniquePeers[u.ID] = peerInfo{
							user:     u,
							name:     fmt.Sprintf("@%s (%s %s)", u.Username, u.FirstName, u.LastName),
							peerType: "User",
						}
					}
				}
			}
		}
	}

	logger.Debug(logger.Sync, "[Int ID %d] Found %d unique private chats across folders", integrationID, len(uniquePeers))

	for _, info := range uniquePeers {
		if info.user.Bot {
			logger.Debug(logger.Sync, "[Trace] Skipping bot: %s", info.name)
			continue
		}

		logger.Debug(logger.Sync, "[Trace] Syncing private chat: %s", info.name)
		contactID, err := m.getOrCreateContact(ctx, info.user, integrationID)
		if err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to ensure contact %s: %v", integrationID, info.name, err)
			continue
		}

		if err := m.SyncHistory(ctx, api, info.user, contactID, integrationID); err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to sync history for %s: %v", integrationID, info.name, err)
		}
	}

	logger.Debug(logger.Sync, "[Int ID %d] Initial sync finished", integrationID)
	return nil
}

func (m *Manager) HandleNewMessage(ctx context.Context, api *tg.Client, u *tg.UpdateNewMessage, users map[int64]*tg.User, integrationID int64) error {
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
		logger.Debug(logger.Sync, "[Int ID %d] Fetching missing user info for %d...", integrationID, userID)
		usersRes, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{
			UserID: userID,
		}})
		if err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to fetch user %d: %v", integrationID, userID, err)
			return nil
		}

		if len(usersRes) > 0 {
			if u, ok := usersRes[0].(*tg.User); ok {
				user = u
			}
		}
	}

	if user == nil {
		logger.Debug(logger.Sync, "[Int ID %d] Could not resolve user %d", integrationID, userID)
		return nil
	}

	contactID, err := m.getOrCreateContact(ctx, user, integrationID)
	if err != nil {
		logger.Debug(logger.Sync, "[Int ID %d] DB Error handling contact: %v", integrationID, err)
		return err
	}

	// Save message
	externalID := fmt.Sprintf("%d", msg.ID)
	_, err = database.DB.Exec("INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		integrationID, contactID, externalID, text, !msg.Out, time.Unix(int64(msg.Date), 0))
	if err != nil {
		logger.Debug(logger.Sync, "[Int ID %d] DB Error saving message: %v", integrationID, err)
		return err
	}

	logger.Debug(logger.Sync, "[Int ID %d] Captured message from @%s", integrationID, user.Username)

	// Background sync history for this user to ensure we didn't miss anything
	go func() {
		if err := m.SyncHistory(ctx, api, user, contactID, integrationID); err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Background sync failed for @%s: %v", integrationID, user.Username, err)
		}
	}()

	return nil
}

func (m *Manager) SyncHistory(ctx context.Context, api *tg.Client, user *tg.User, contactID, integrationID int64) error {
	logger.Debug(logger.Sync, "[Int ID %d] Syncing history for @%s...", integrationID, user.Username)

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
		externalID := fmt.Sprintf("%d", msg.ID)
		res, err := database.DB.Exec("INSERT OR IGNORE INTO messages (integration_id, contact_id, external_id, text, is_incoming, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
			integrationID, contactID, externalID, msg.Message, !msg.Out, time.Unix(int64(msg.Date), 0))
		if err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] DB Error during sync for @%s: %v", integrationID, user.Username, err)
			continue
		}

		rows, _ := res.RowsAffected()
		if rows > 0 {
			newMsgs++
		}
	}

	if newMsgs > 0 {
		logger.Debug(logger.Sync, "[Int ID %d] Finished sync for @%s: saved %d new messages", integrationID, user.Username, newMsgs)
	}
	return nil
}
