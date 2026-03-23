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
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"hr-sorter/internal/database"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
)

type Manager struct {
	clients   map[int64]*telegram.Client
	cancels   map[int64]context.CancelFunc
	codeChans map[int64]chan string
	passChans map[int64]chan string
	mu        sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		clients:   make(map[int64]*telegram.Client),
		cancels:   make(map[int64]context.CancelFunc),
		codeChans: make(map[int64]chan string),
		passChans: make(map[int64]chan string),
	}
}

func (m *Manager) StartIntegration(ctx context.Context, integration models.Integration) error {
	m.mu.Lock()
	if _, running := m.cancels[integration.ID]; running {
		m.mu.Unlock()
		logger.Debug(logger.Telegram, "Integration %s is already running", integration.Identifier)
		return nil
	}

	// Create a sub-context for this specific integration
	intCtx, cancel := context.WithCancel(ctx)
	m.cancels[integration.ID] = cancel
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		delete(m.cancels, integration.ID)
		delete(m.codeChans, integration.ID)
		delete(m.passChans, integration.ID)
		m.mu.Unlock()
	}()

	logger.Debug(logger.Telegram, "Starting integration %s (ID: %d, Path: %s, API ID: %d)", integration.Identifier, integration.ID, integration.SessionPath, integration.APIID)
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
	logger.Debug(logger.Telegram, "Using session file: %s", sessionPath)

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

	// Setup Auth Flow
	codeChan := make(chan string, 1)
	passChan := make(chan string, 1)
	m.mu.Lock()
	m.codeChans[integration.ID] = codeChan
	m.passChans[integration.ID] = passChan
	m.mu.Unlock()

	// Custom authenticator that handles both code and password
	a := &codeAuthenticator{
		phone:         integration.Identifier,
		codeChan:      codeChan,
		passChan:      passChan,
		integrationID: integration.ID,
	}

	flow := auth.NewFlow(a, auth.SendCodeOptions{})

	logger.Debug(logger.Telegram, "Client initialized, entering Run loop...")

	return client.Run(intCtx, func(ctx context.Context) error {
		logger.Debug(logger.Telegram, "[Int ID %d] Calling client.Auth().IfNecessary (auth flow starts)", integration.ID)
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			logger.Debug(logger.Telegram, "[Int ID %d] Auth failed: %v", integration.ID, err)
			database.DB.Exec("UPDATE integrations SET status = 'pending_auth' WHERE id = ?", integration.ID)
			return fmt.Errorf("auth failed: %w", err)
		}

		logger.Debug(logger.Telegram, "[Int ID %d] Auth succeeded! Updating status to active.", integration.ID)
		database.DB.Exec("UPDATE integrations SET status = 'active' WHERE id = ?", integration.ID)

		logger.Debug(logger.Telegram, "Client loop running for %s", integration.Identifier)
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

func (m *Manager) SubmitCode(id int64, code string) bool {
	m.mu.RLock()
	ch, ok := m.codeChans[id]
	m.mu.RUnlock()
	if ok {
		ch <- code
		return true
	}
	return false
}

func (m *Manager) SubmitPassword(id int64, password string) bool {
	m.mu.RLock()
	ch, ok := m.passChans[id]
	m.mu.RUnlock()
	if ok {
		ch <- password
		return true
	}
	return false
}

// codeAuthenticator implements auth.UserAuthenticator
type codeAuthenticator struct {
	phone         string
	codeChan      chan string
	passChan      chan string
	integrationID int64
}

func (a *codeAuthenticator) Phone(ctx context.Context) (string, error) {
	return a.phone, nil
}

func (a *codeAuthenticator) Password(ctx context.Context) (string, error) {
	logger.Debug(logger.Telegram, "[Int ID %d] Auth flow triggered - PASSWORD needed. Updating status to awaiting_password.", a.integrationID)
	database.DB.Exec("UPDATE integrations SET status = 'awaiting_password' WHERE id = ?", a.integrationID)

	select {
	case password := <-a.passChan:
		logger.Debug(logger.Telegram, "[Int ID %d] Received password from UI. Submitting to Telegram...", a.integrationID)
		return password, nil
	case <-ctx.Done():
		logger.Debug(logger.Telegram, "[Int ID %d] Context cancelled while waiting for password", a.integrationID)
		return "", ctx.Err()
	case <-time.After(5 * time.Minute):
		logger.Debug(logger.Telegram, "[Int ID %d] Password entry timeout (5 min)", a.integrationID)
		return "", fmt.Errorf("auth timeout")
	}
}

func (a *codeAuthenticator) AcceptTermsOfService(ctx context.Context, tos tg.HelpTermsOfService) error {
	return nil
}

func (a *codeAuthenticator) SignUp(ctx context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, nil
}

func (a *codeAuthenticator) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	logger.Debug(logger.Telegram, "[Int ID %d] Auth flow triggered. Has code: %v, PhoneChanged: %v",
		a.integrationID, sentCode != nil, sentCode != nil && sentCode.PhoneCodeHash != "")
	logger.Debug(logger.Telegram, "[Int ID %d] Auth requested code. Updating status to awaiting_code.", a.integrationID)
	database.DB.Exec("UPDATE integrations SET status = 'awaiting_code' WHERE id = ?", a.integrationID)

	select {
	case code := <-a.codeChan:
		logger.Debug(logger.Telegram, "[Int ID %d] Received code from UI. Submitting code to Telegram...", a.integrationID)
		return code, nil
	case <-ctx.Done():
		logger.Debug(logger.Telegram, "[Int ID %d] Context cancelled while waiting for code", a.integrationID)
		return "", ctx.Err()
	case <-time.After(5 * time.Minute):
		logger.Debug(logger.Telegram, "[Int ID %d] Code entry timeout (5 min)", a.integrationID)
		return "", fmt.Errorf("auth timeout")
	}
}

func (m *Manager) StopIntegration(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cancel, ok := m.cancels[id]; ok {
		logger.Debug(logger.Telegram, "Stopping integration ID %d", id)
		cancel()
		delete(m.cancels, id)
	}
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
	logger.Debug(logger.Telegram, "[Int ID %d] Starting initial verification...", integrationID)
	// Verify session first
	me, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err != nil {
		logger.Debug(logger.Telegram, "[Int ID %d] Session verification failed: %v", integrationID, err)
		return fmt.Errorf("failed to verify session: %w", err)
	}
	if len(me) > 0 {
		if u, ok := me[0].(*tg.User); ok {
			logger.Debug(logger.Telegram, "[Int ID %d] Session verified as @%s (%s %s). Updating status to active.", integrationID, u.Username, u.FirstName, u.LastName)
			// Auto-activate if verified
			database.DB.Exec("UPDATE integrations SET status = 'active' WHERE id = ?", integrationID)
		}
	} else {
		logger.Debug(logger.Telegram, "[Int ID %d] No user info returned from Telegram (unauthorized)", integrationID)
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
