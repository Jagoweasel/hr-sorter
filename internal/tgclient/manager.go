package tgclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
	"github.com/jmoiron/sqlx"
	"hr-sorter/internal/logger"
	"hr-sorter/internal/models"
	"hr-sorter/internal/repository"
	"hr-sorter/internal/streaming"
)

type Manager struct {
	clients   map[int64]*telegram.Client
	apis      map[int64]*tg.Client
	cancels   map[int64]context.CancelFunc
	codeChans map[int64]chan string
	passChans map[int64]chan string
	mu        sync.RWMutex

	db             *sqlx.DB
	conRepo        *repository.ContactRepository
	msgRepo        *repository.MessageRepository
	stRepo         *repository.StateRepository
	intRepo        *repository.IntegrationRepository
	logBroadcaster *streaming.LogBroadcaster
}

func NewManager(db *sqlx.DB, conRepo *repository.ContactRepository, msgRepo *repository.MessageRepository, stRepo *repository.StateRepository, intRepo *repository.IntegrationRepository, lb *streaming.LogBroadcaster) *Manager {
	return &Manager{
		clients:        make(map[int64]*telegram.Client),
		apis:           make(map[int64]*tg.Client),
		cancels:        make(map[int64]context.CancelFunc),
		codeChans:      make(map[int64]chan string),
		passChans:      make(map[int64]chan string),
		db:             db,
		conRepo:        conRepo,
		msgRepo:        msgRepo,
		stRepo:         stRepo,
		intRepo:        intRepo,
		logBroadcaster: lb,
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
		delete(m.apis, integration.ID)
		m.mu.Unlock()
	}()

	logger.Debug(logger.Telegram, "Starting integration %s (ID: %d, Path: %s, API ID: %d)", integration.Identifier, integration.ID, integration.SessionPath, integration.APIID)
	if integration.SessionPath == "" {
		return fmt.Errorf("no session path for integration %s", integration.Identifier)
	}

	if integration.APIID == 0 || integration.APIHash == "" {
		return fmt.Errorf("missing API credentials for integration %s", integration.Identifier)
	}

	// Setup Update Dispatcher and Manager
	dispatcher := tg.NewUpdateDispatcher()
	var api *tg.Client

	// Create updates manager for reliable delivery
	updateManager := updates.New(updates.Config{
		Handler:      dispatcher,
		Storage:      &dbStateStorage{integrationID: integration.ID, stRepo: m.stRepo, conRepo: m.conRepo},
		AccessHasher: &dbStateStorage{integrationID: integration.ID, stRepo: m.stRepo, conRepo: m.conRepo},
	})

	// Detailed logging for ALL updates
	dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		logger.Debug(logger.Sync, "[Int ID %d] Received NewMessage update (ID: %d)", integration.ID, u.Message.GetID())
		return m.HandleNewMessage(ctx, api, u.Message, e.Users, integration.ID)
	})

	dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		logger.Debug(logger.Sync, "[Int ID %d] Received NewChannelMessage update (ID: %d)", integration.ID, u.Message.GetID())
		return m.HandleNewMessage(ctx, api, u.Message, e.Users, integration.ID)
	})

	dispatcher.OnNewScheduledMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewScheduledMessage) error {
		logger.Debug(logger.Sync, "[Int ID %d] Received NewScheduledMessage update", integration.ID)
		return m.HandleNewMessage(ctx, api, u.Message, e.Users, integration.ID)
	})

	// Ensure absolute path and use forward slashes for cross-platform compatibility
	sessionPath, _ := filepath.Abs(integration.SessionPath)
	sessionPath = filepath.ToSlash(sessionPath)
	logger.Debug(logger.Telegram, "Using session file: %s", sessionPath)

	client := telegram.NewClient(integration.APIID, integration.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: sessionPath,
		},
		UpdateHandler: updateManager,
	})

	api = tg.NewClient(client)
	m.mu.Lock()
	m.apis[integration.ID] = api
	m.mu.Unlock()

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
		intRepo:       m.intRepo,
	}

	flow := auth.NewFlow(a, auth.SendCodeOptions{})

	logger.Debug(logger.Telegram, "Client initialized, entering Run loop...")

	// Wrap in a retry loop for automatic reconnection
	for {
		err := client.Run(intCtx, func(ctx context.Context) error {
			logger.Debug(logger.Telegram, "[Int ID %d] Calling client.Auth().IfNecessary (auth flow starts)", integration.ID)
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				logger.Debug(logger.Telegram, "[Int ID %d] Auth failed: %v", integration.ID, err)
				m.intRepo.UpdateStatus(ctx, integration.ID, "pending_auth")
				return fmt.Errorf("auth failed: %w", err)
			}

			logger.Debug(logger.Telegram, "[Int ID %d] Auth succeeded! Updating status to active.", integration.ID)
			m.intRepo.UpdateStatus(ctx, integration.ID, "active")

			logger.Debug(logger.Telegram, "Client loop running for %s", integration.Identifier)
			log.Printf("[Integration %s] [RUNNING] Logged in successfully.", integration.Identifier)

			// Start update manager syncing
			go func() {
				if err := updateManager.Run(ctx, api, meID(ctx, api), updates.AuthOptions{
					OnStart: func(ctx context.Context) {
						logger.Debug(logger.Sync, "[Integration %s] Update manager started syncing.", integration.Identifier)
					},
				}); err != nil && ctx.Err() == nil {
					log.Printf("[Integration %s] [ERROR] Update manager failed: %v", integration.Identifier, err)
				}
			}()

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

		if err == nil || errors.Is(err, context.Canceled) {
			break
		}

		log.Printf("[Integration %s] [RETRY] Client exited with error: %v. Retrying in 5s...", integration.Identifier, err)
		select {
		case <-intCtx.Done():
			return intCtx.Err()
		case <-time.After(5 * time.Second):
			continue
		}
	}
	return nil
}

func meID(ctx context.Context, api *tg.Client) int64 {
	me, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUserSelf{}})
	if err == nil && len(me) > 0 {
		if u, ok := me[0].(*tg.User); ok {
			return u.ID
		}
	}
	return 0
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
	intRepo       *repository.IntegrationRepository
}

func (a *codeAuthenticator) Phone(ctx context.Context) (string, error) {
	return a.phone, nil
}

func (a *codeAuthenticator) Password(ctx context.Context) (string, error) {
	logger.Debug(logger.Telegram, "[Int ID %d] Auth flow triggered - PASSWORD needed. Updating status to awaiting_password.", a.integrationID)
	a.intRepo.UpdateStatus(ctx, a.integrationID, "awaiting_password")

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
	a.intRepo.UpdateStatus(ctx, a.integrationID, "awaiting_code")

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

func (m *Manager) SendMessage(ctx context.Context, integrationID int64, contactID string, accessHash int64, username string, text string) (int, error) {
	logger.Debug(logger.Messaging, "[TG] [Int ID %d] Attempting to send message to %s (hash: %d, user: %s)", integrationID, contactID, accessHash, username)

	m.mu.RLock()
	api, ok := m.apis[integrationID]
	m.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("no active API client for integration %d (is it logged in?)", integrationID)
	}

	// Double check api isn't nil (safety against racing/partial init)
	if api == nil {
		return 0, fmt.Errorf("API client for integration %d exists but is not yet initialized", integrationID)
	}

	// Parse contactID to int64
	var targetID int64
	fmt.Sscanf(contactID, "%d", &targetID)

	var peer tg.InputPeerClass
	if strings.HasPrefix(username, "channel_") {
		peer = &tg.InputPeerChannel{ChannelID: targetID, AccessHash: accessHash}
	} else if strings.HasPrefix(username, "chat_") {
		peer = &tg.InputPeerChat{ChatID: targetID}
	} else {
		peer = &tg.InputPeerUser{UserID: targetID, AccessHash: accessHash}
	}

	logger.Debug(logger.Messaging, "[TG] [Int ID %d] Using peer: %T", integrationID, peer)

	res, err := api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:     peer,
		Message:  text,
		RandomID: time.Now().UnixNano(),
	})
	if err != nil {
		logger.Debug(logger.Messaging, "[TG] [Int ID %d] Send error: %v", integrationID, err)
		return 0, err
	}

	switch u := res.(type) {
	case *tg.UpdateShortSentMessage:
		logger.Debug(logger.Messaging, "[TG] [Int ID %d] Sent successfully (ShortSent, ID: %d)", integrationID, u.ID)
		return u.ID, nil
	case *tg.Updates:
		for _, upd := range u.Updates {
			if m, ok := upd.(*tg.UpdateNewMessage); ok {
				if msg, ok := m.Message.(*tg.Message); ok {
					logger.Debug(logger.Messaging, "[TG] [Int ID %d] Sent successfully (NewMessage, ID: %d)", integrationID, msg.ID)
					return msg.ID, nil
				}
			}
		}
	default:
		logger.Debug(logger.Messaging, "[TG] [Int ID %d] Sent but received unknown update type: %T", integrationID, u)
	}

	return 0, nil
}

func (m *Manager) StopIntegration(id int64) {
	m.mu.Lock()
	defer m.mu.RUnlock()
	if cancel, ok := m.cancels[id]; ok {
		logger.Debug(logger.Telegram, "Stopping integration ID %d", id)
		cancel()
		delete(m.cancels, id)
	}
}

func (m *Manager) GetActiveCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.cancels)
}

func (m *Manager) getOrCreateContact(ctx context.Context, user *tg.User, integrationID int64) (int64, error) {
	externalID := fmt.Sprintf("%d", user.ID)
	logger.Debug(logger.Sync, "[Int ID %d] getOrCreateContact: %s %s (@%s), ID: %s, AccessHash: %d", integrationID, user.FirstName, user.LastName, user.Username, externalID, user.AccessHash)

	if err := m.conRepo.UpsertTGContact(ctx, integrationID, externalID, user.FirstName, user.LastName, user.Username, user.AccessHash); err != nil {
		return 0, err
	}

	return m.conRepo.GetIDByExternalID(ctx, "tg", externalID)
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
			m.intRepo.UpdateStatus(ctx, integrationID, "active")
		}
	} else {
		logger.Debug(logger.Telegram, "[Int ID %d] No user info returned from Telegram (unauthorized)", integrationID)
	}

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
		var chats = make(map[int64]tg.ChatClass)
		var messages = make(map[int]tg.MessageClass)
		var dialogs []tg.DialogClass

		switch d := res.(type) {
		case *tg.MessagesDialogs:
			dialogs = d.Dialogs
			for _, uClass := range d.Users {
				if u, ok := uClass.(*tg.User); ok {
					users[u.ID] = u
				}
			}
			for _, cClass := range d.Chats {
				chats[cClass.GetID()] = cClass
			}
			for _, mClass := range d.Messages {
				if m, ok := mClass.AsNotEmpty(); ok {
					messages[m.GetID()] = mClass
				}
			}
		case *tg.MessagesDialogsSlice:
			dialogs = d.Dialogs
			for _, uClass := range d.Users {
				if u, ok := uClass.(*tg.User); ok {
					users[u.ID] = u
				}
			}
			for _, cClass := range d.Chats {
				chats[cClass.GetID()] = cClass
			}
			for _, mClass := range d.Messages {
				if m, ok := mClass.AsNotEmpty(); ok {
					messages[m.GetID()] = mClass
				}
			}
		}

		logger.Debug(logger.Sync, "[Int ID %d] Found %d total dialogs in %s", integrationID, len(dialogs), folderName)

		// Start a transaction for this folder's dialogs
		tx, err := m.db.BeginTxx(ctx, nil)
		if err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to start tx for folder sync: %v", integrationID, err)
			continue
		}
		defer tx.Rollback()

		for _, dClass := range dialogs {
			d, ok := dClass.(*tg.Dialog)
			if !ok {
				continue
			}

			var contactID int64
			var peerUser *tg.User
			var peerName string

			switch p := d.Peer.(type) {
			case *tg.PeerUser:
				if u, ok := users[p.UserID]; ok {
					peerUser = u
					peerName = fmt.Sprintf("@%s (%s %s)", u.Username, u.FirstName, u.LastName)
					if u.Bot {
						continue // Skip bots
					}
				}
			case *tg.PeerChat:
				if c, ok := chats[p.ChatID]; ok {
					title := ""
					if chat, ok := c.(*tg.Chat); ok {
						title = chat.Title
					}
					peerUser = &tg.User{
						ID:        p.ChatID,
						FirstName: title,
						Username:  fmt.Sprintf("chat_%d", p.ChatID),
					}
					peerName = title
				}
			case *tg.PeerChannel:
				if c, ok := chats[p.ChannelID]; ok {
					title := ""
					var accessHash int64
					if channel, ok := c.(*tg.Channel); ok {
						title = channel.Title
						accessHash = channel.AccessHash
					}
					peerUser = &tg.User{
						ID:         p.ChannelID,
						FirstName:  title,
						Username:   fmt.Sprintf("channel_%d", p.ChannelID),
						AccessHash: accessHash,
					}
					peerName = title
				}
			}

			if peerUser != nil {
				externalID := fmt.Sprintf("%d", peerUser.ID)
				// Upsert contact in tx
				if err := m.conRepo.UpsertTGContactExt(ctx, tx, integrationID, externalID, peerUser.FirstName, peerUser.LastName, peerUser.Username, peerUser.AccessHash); err != nil {
					logger.Debug(logger.Sync, "[Int ID %d] Failed to upsert contact %s: %v", integrationID, peerName, err)
					continue
				}

				// Get internal contact ID
				var cID int64
				err := tx.GetContext(ctx, &cID, "SELECT id FROM contacts WHERE platform = 'tg' AND external_id = ?", externalID)
				if err == nil {
					contactID = cID
					// Save the top message if we have it
					if msg, ok := messages[d.TopMessage]; ok {
						if mObj, ok := msg.(*tg.Message); ok {
							extMsgID := fmt.Sprintf("%d", mObj.ID)
							ts := time.Unix(int64(mObj.Date), 0).UTC().Format("2006-01-02 15:04:05")
							m.msgRepo.CreateExt(ctx, tx, integrationID, contactID, extMsgID, mObj.Message, !mObj.Out, ts)
						}
					}
				}
			}
		}

		if err := tx.Commit(); err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] Failed to commit folder sync tx: %v", integrationID, err)
		}
	}

	logger.Debug(logger.Sync, "[Int ID %d] Initial sync finished (top messages only)", integrationID)
	return nil
}

func (m *Manager) HandleNewMessage(ctx context.Context, api *tg.Client, msgClass tg.MessageClass, users map[int64]*tg.User, integrationID int64) error {
	msg, ok := msgClass.(*tg.Message)
	if !ok {
		if _, service := msgClass.(*tg.MessageService); service {
			logger.Debug(logger.Sync, "[Int ID %d] Received Service Message (ignoring for now)", integrationID)
		} else {
			logger.Debug(logger.Sync, "[Int ID %d] Received unknown message class: %T", integrationID, msgClass)
		}
		return nil
	}

	var userID int64
	var peerName string

	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		userID = p.UserID
	case *tg.PeerChat:
		userID = p.ChatID
		peerName = fmt.Sprintf("Chat %d", p.ChatID)
	case *tg.PeerChannel:
		userID = p.ChannelID
		peerName = fmt.Sprintf("Channel %d", p.ChannelID)
	default:
		return nil
	}

	text := msg.Message

	var user *tg.User
	if peerName == "" { // It's a User peer
		user, ok = users[userID]
		if !ok {
			logger.Debug(logger.Sync, "[Int ID %d] Fetching missing user info for %d via MessagesGetMessages...", integrationID, userID)
			// Try to get message info to force resolve entities
			// If we have no entities, we can try to fetch the message we just got
			// This often returns the missing users in the response
			res, err := api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: msg.ID}})
			if err == nil {
				logger.Debug(logger.Sync, "[Int ID %d] MessagesGetMessages succeeded, looking for user %d in entities...", integrationID, userID)
				switch mres := res.(type) {
				case *tg.MessagesMessages:
					for _, uClass := range mres.Users {
						if u, ok := uClass.(*tg.User); ok {
							logger.Debug(logger.Sync, "[Int ID %d] Found user %d (@%s) in entities", integrationID, u.ID, u.Username)
							if u.ID == userID {
								user = u
							}
						}
					}
				case *tg.MessagesMessagesSlice:
					for _, uClass := range mres.Users {
						if u, ok := uClass.(*tg.User); ok {
							logger.Debug(logger.Sync, "[Int ID %d] Found user %d (@%s) in entities", integrationID, u.ID, u.Username)
							if u.ID == userID {
								user = u
							}
						}
					}
				}
			} else {
				logger.Debug(logger.Sync, "[Int ID %d] MessagesGetMessages failed: %v", integrationID, err)
			}

			if user == nil {
				// Final fallback: direct fetch (might fail without access hash)
				usersRes, err := api.UsersGetUsers(ctx, []tg.InputUserClass{&tg.InputUser{
					UserID: userID,
				}})
				if err == nil && len(usersRes) > 0 {
					if u, ok := usersRes[0].(*tg.User); ok {
						user = u
					}
				}
			}
		}
	}

	// For non-user peers, we might want to create a synthetic contact or just handle it as a system user
	if user == nil && peerName != "" {
		// Create a placeholder user object for groups/channels to satisfy getOrCreateContact
		user = &tg.User{
			ID:        userID,
			FirstName: peerName,
			Username:  peerName,
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
	ts := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02 15:04:05")
	err = m.msgRepo.Create(ctx, integrationID, contactID, externalID, text, !msg.Out, ts)
	if err != nil {
		logger.Debug(logger.Sync, "[Int ID %d] DB Error saving message: %v", integrationID, err)
		return err
	}

	logger.Debug(logger.Sync, "[Int ID %d] Captured message from %s", integrationID, user.FirstName)

	// Background sync history for this user to ensure we didn't miss anything (only for users)
	if peerName == "" {
		go func() {
			if err := m.SyncHistory(ctx, api, user, contactID, integrationID); err != nil {
				logger.Debug(logger.Sync, "[Int ID %d] Background sync failed for %s: %v", integrationID, user.FirstName, err)
			}
		}()
	}

	return nil
}

func (m *Manager) SyncHistory(ctx context.Context, api *tg.Client, user *tg.User, contactID, integrationID int64) error {
	logger.Debug(logger.Sync, "[Int ID %d] Syncing history for %s...", integrationID, user.FirstName)

	// Telegram might return different types of message lists
	var messages []tg.MessageClass

	// If user is missing access hash, try to fetch it from DB
	if user.AccessHash == 0 {
		stored, err := m.conRepo.GetByID(ctx, contactID)
		if err == nil {
			user.AccessHash = stored.AccessHash
		}
	}

	// Determine peer type
	var peer tg.InputPeerClass
	if strings.HasPrefix(user.Username, "channel_") {
		peer = &tg.InputPeerChannel{ChannelID: user.ID, AccessHash: user.AccessHash}
	} else if strings.HasPrefix(user.Username, "chat_") {
		peer = &tg.InputPeerChat{ChatID: user.ID}
	} else {
		peer = &tg.InputPeerUser{UserID: user.ID, AccessHash: user.AccessHash}
	}

	history, err := api.MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{
		Peer:  peer,
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
	// Start a transaction for batch insertion
	tx, err := m.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	for _, mClass := range messages {
		msg, ok := mClass.(*tg.Message)
		if !ok {
			continue
		}

		// Use INSERT OR IGNORE to avoid duplicates if message was already captured live
		externalID := fmt.Sprintf("%d", msg.ID)
		ts := time.Unix(int64(msg.Date), 0).UTC().Format("2006-01-02 15:04:05")

		// Check if message exists before inserting (to avoid unnecessary disk writes in WAL)
		// Actually INSERT OR IGNORE is fine since we are in a transaction
		if err := m.msgRepo.CreateExt(ctx, tx, integrationID, contactID, externalID, msg.Message, !msg.Out, ts); err != nil {
			logger.Debug(logger.Sync, "[Int ID %d] DB Error during sync for @%s: %v", integrationID, user.Username, err)
			continue
		}
		newMsgs++
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	if newMsgs > 0 {
		logger.Debug(logger.Sync, "[Int ID %d] Finished sync for @%s: saved %d new messages", integrationID, user.Username, newMsgs)
	}
	return nil
}

type dbStateStorage struct {
	integrationID int64
	stRepo        *repository.StateRepository
	conRepo       *repository.ContactRepository
}

func (s *dbStateStorage) GetState(ctx context.Context, userID int64) (updates.State, bool, error) {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		return updates.State{}, false, nil
	}
	logger.Debug(logger.Sync, "[Int ID %d] Loaded update state: PTS=%d, QTS=%d, Seq=%d", s.integrationID, state.Pts, state.Qts, state.Seq)
	return updates.State{Pts: state.Pts, Qts: state.Qts, Seq: state.Seq, Date: state.Date}, true, nil
}

func (s *dbStateStorage) SetState(ctx context.Context, userID int64, state updates.State) error {
	logger.Debug(logger.Sync, "[Int ID %d] Saving update state: PTS=%d, QTS=%d, Seq=%d", s.integrationID, state.Pts, state.Qts, state.Seq)
	return s.stRepo.UpsertTGState(ctx, repository.TGState{
		IntegrationID: s.integrationID,
		Pts:           state.Pts,
		Qts:           state.Qts,
		Seq:           state.Seq,
		Date:          state.Date,
	})
}

func (s *dbStateStorage) SetPts(ctx context.Context, userID int64, pts int) error {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		state = &repository.TGState{IntegrationID: s.integrationID}
	}
	state.Pts = pts
	return s.stRepo.UpsertTGState(ctx, *state)
}

func (s *dbStateStorage) SetQts(ctx context.Context, userID int64, qts int) error {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		state = &repository.TGState{IntegrationID: s.integrationID}
	}
	state.Qts = qts
	return s.stRepo.UpsertTGState(ctx, *state)
}

func (s *dbStateStorage) SetDate(ctx context.Context, userID int64, date int) error {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		state = &repository.TGState{IntegrationID: s.integrationID}
	}
	state.Date = date
	return s.stRepo.UpsertTGState(ctx, *state)
}

func (s *dbStateStorage) SetSeq(ctx context.Context, userID int64, seq int) error {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		state = &repository.TGState{IntegrationID: s.integrationID}
	}
	state.Seq = seq
	return s.stRepo.UpsertTGState(ctx, *state)
}

func (s *dbStateStorage) SetDateSeq(ctx context.Context, userID int64, date, seq int) error {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		state = &repository.TGState{IntegrationID: s.integrationID}
	}
	state.Date = date
	state.Seq = seq
	return s.stRepo.UpsertTGState(ctx, *state)
}

func (s *dbStateStorage) GetChannelPts(ctx context.Context, userID, channelID int64) (int, bool, error) {
	pts, err := s.stRepo.GetChannelPts(ctx, s.integrationID, channelID)
	if err != nil {
		return 0, false, nil
	}
	return pts, true, nil
}

func (s *dbStateStorage) SetChannelPts(ctx context.Context, userID, channelID int64, pts int) error {
	return s.stRepo.UpsertChannelPts(ctx, s.integrationID, channelID, pts)
}

func (s *dbStateStorage) ForEachChannels(ctx context.Context, userID int64, f func(ctx context.Context, channelID int64, pts int) error) error {
	channels, err := s.stRepo.GetAllChannels(ctx, s.integrationID)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := f(ctx, ch.ChannelID, ch.Pts); err != nil {
			return err
		}
	}
	return nil
}

func (s *dbStateStorage) SetChannelAccessHash(ctx context.Context, userID, channelID, accessHash int64) error {
	// We use contacts table for access hashes
	return s.conRepo.UpsertTGContactExt(ctx, s.stRepo.GetDB(), s.integrationID, fmt.Sprintf("%d", channelID), "", "", fmt.Sprintf("channel_%d", channelID), accessHash)
}

func (s *dbStateStorage) GetChannelAccessHash(ctx context.Context, userID, channelID int64) (int64, bool, error) {
	id, err := s.conRepo.GetIDByExternalID(ctx, "tg", fmt.Sprintf("%d", channelID))
	if err != nil {
		return 0, false, nil
	}
	contact, err := s.conRepo.GetByID(ctx, id)
	if err != nil {
		return 0, false, nil
	}
	return contact.AccessHash, contact.AccessHash != 0, nil
}

func (s *dbStateStorage) LoadState(ctx context.Context) (updates.State, error) {
	state, err := s.stRepo.GetTGState(ctx, s.integrationID)
	if err != nil {
		return updates.State{}, nil
	}
	return updates.State{Pts: state.Pts, Qts: state.Qts, Seq: state.Seq, Date: state.Date}, nil
}

func (s *dbStateStorage) SaveState(ctx context.Context, state updates.State) error {
	return s.stRepo.UpsertTGState(ctx, repository.TGState{
		IntegrationID: s.integrationID,
		Pts:           state.Pts,
		Qts:           state.Qts,
		Seq:           state.Seq,
		Date:          state.Date,
	})
}
