package bridge

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

type whatsmeowAdapter struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
	rawDB     *sql.DB
}

func newWhatsmeowAdapter(ctx context.Context, dataDir string, deviceName string) (waAdapter, bool, error) {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, false, err
	}

	dbPath := filepath.ToSlash(filepath.Join(dataDir, "whatsmeow.db"))
	rawDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_foreign_keys=on", dbPath))
	if err != nil {
		return nil, false, fmt.Errorf("open sqlite store: %w", err)
	}
	if _, err := rawDB.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		_ = rawDB.Close()
		return nil, false, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	container := sqlstore.NewWithDB(rawDB, "sqlite3", waLog.Noop)
	if err := container.Upgrade(ctx); err != nil {
		_ = container.Close()
		return nil, false, fmt.Errorf("upgrade whatsmeow store: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, false, fmt.Errorf("get first device: %w", err)
	}
	hadSession := deviceStore.ID != nil
	deviceStore.PushName = deviceName

	client := whatsmeow.NewClient(deviceStore, waLog.Noop)
	client.EnableAutoReconnect = false
	client.InitialAutoReconnect = false
	client.DisableLoginAutoReconnect = true
	client.EmitAppStateEventsOnFullSync = false
	client.ManualHistorySyncDownload = true

	return &whatsmeowAdapter{
		client:    client,
		container: container,
		rawDB:     rawDB,
	}, hadSession, nil
}

func (a *whatsmeowAdapter) AddEventHandler(handler func(any)) uint32 {
	return a.client.AddEventHandler(handler)
}

func (a *whatsmeowAdapter) GetQRChannel(ctx context.Context) (<-chan qrItem, error) {
	source, err := a.client.GetQRChannel(ctx)
	if err != nil {
		return nil, err
	}
	out := make(chan qrItem, 8)
	go func() {
		defer close(out)
		for item := range source {
			out <- qrItem{
				Event:   item.Event,
				Code:    item.Code,
				Error:   item.Error,
				Timeout: item.Timeout,
			}
		}
	}()
	return out, nil
}

func (a *whatsmeowAdapter) ConnectContext(ctx context.Context) error {
	return a.client.ConnectContext(ctx)
}

func (a *whatsmeowAdapter) ReconnectAfterLogin(ctx context.Context) error {
	a.client.Disconnect()
	return a.client.ConnectContext(ctx)
}

func (a *whatsmeowAdapter) SendText(ctx context.Context, phone string, text string, clientMsgId string) (sendTextResult, error) {
	jid, err := a.resolveJID(ctx, phone)
	if err != nil {
		return sendTextResult{}, err
	}
	jid = a.preferredSendJID(ctx, jid)
	route := sendTextResult{
		RecipientJID:    jid.String(),
		RecipientServer: string(jid.Server),
		UsedLID:         jid.Server == types.HiddenUserServer,
	}

	resp, err := a.client.SendMessage(ctx, jid, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return route, err
	}
	route.ServerMessageID = string(resp.ID)
	return route, nil
}

func (a *whatsmeowAdapter) GetContacts(ctx context.Context) ([]contactInfo, error) {
	contacts, err := a.client.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]contactInfo, 0, len(contacts))
	for jid, info := range contacts {
		out = appendContact(ctx, out, a.preferredContactJID, jid, info.FirstName, info.FullName, info.PushName, info.BusinessName)
	}
	if len(out) == 0 {
		fallback, err := a.getContactsByMainJID(ctx)
		if err != nil {
			return nil, err
		}
		out = fallback
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (a *whatsmeowAdapter) GetGroups(ctx context.Context) ([]groupInfo, error) {
	groups, err := a.client.GetJoinedGroups(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]groupInfo, 0, len(groups))
	for _, group := range groups {
		if group == nil || group.JID.IsEmpty() {
			continue
		}
		name := firstNonEmpty(group.Name, group.JID.User)
		out = append(out, groupInfo{
			JID:              group.JID.String(),
			Name:             name,
			ParticipantCount: group.ParticipantCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (a *whatsmeowAdapter) ResolveJID(ctx context.Context, to string) (string, error) {
	jid, err := a.resolveJID(ctx, to)
	if err != nil {
		return "", err
	}
	return jid.String(), nil
}

func (a *whatsmeowAdapter) GetUserInfo(ctx context.Context, jidStrings []string) ([]userInfoResult, error) {
	jids := make([]types.JID, 0, len(jidStrings))
	inputByJID := make(map[string]string, len(jidStrings))
	results := make([]userInfoResult, 0, len(jidStrings))
	for _, jidString := range jidStrings {
		jid, err := a.resolveJID(ctx, jidString)
		if err != nil {
			results = append(results, userInfoResult{JID: jidString, Found: false, Error: err.Error()})
			continue
		}
		jids = append(jids, jid)
		inputByJID[jid.String()] = jidString
	}
	if len(jids) == 0 {
		return results, nil
	}
	infoByJID, err := a.client.GetUserInfo(ctx, jids)
	if err != nil {
		return nil, err
	}
	for _, jid := range jids {
		info, ok := infoByJID[jid]
		item := userInfoResult{
			JID:   jid.String(),
			Found: ok,
		}
		if !ok {
			if input := inputByJID[jid.String()]; input != "" {
				item.JID = input
			}
			results = append(results, item)
			continue
		}
		item.Status = info.Status
		item.PictureID = info.PictureID
		if !info.LID.IsEmpty() {
			item.LID = info.LID.String()
		}
		if info.VerifiedName != nil && info.VerifiedName.Details != nil {
			item.VerifiedName = info.VerifiedName.Details.GetVerifiedName()
		}
		for _, device := range info.Devices {
			item.Devices = append(item.Devices, device.String())
		}
		results = append(results, item)
	}
	return results, nil
}

func (a *whatsmeowAdapter) GetProfilePictureInfo(ctx context.Context, jidString string) (profilePictureResult, error) {
	jid, err := a.resolveJID(ctx, jidString)
	if err != nil {
		return profilePictureResult{JID: jidString, Found: false, Error: err.Error()}, nil
	}
	info, err := a.client.GetProfilePictureInfo(ctx, jid, nil)
	if err != nil {
		return profilePictureResult{JID: jid.String(), Found: false, Error: err.Error()}, nil
	}
	if info == nil {
		return profilePictureResult{JID: jid.String(), Found: false, Error: "profile picture has not changed or is not available"}, nil
	}
	return profilePictureResult{
		JID:        jid.String(),
		Found:      true,
		URL:        info.URL,
		ID:         info.ID,
		Type:       info.Type,
		DirectPath: info.DirectPath,
		Hash:       base64.StdEncoding.EncodeToString(info.Hash),
	}, nil
}

func (a *whatsmeowAdapter) MarkRead(ctx context.Context, chatString string, ids []string, senderString string) error {
	chat, err := a.resolveJID(ctx, chatString)
	if err != nil {
		return err
	}
	sender := types.EmptyJID
	if strings.TrimSpace(senderString) != "" {
		sender, err = a.resolveJID(ctx, senderString)
		if err != nil {
			return err
		}
	}
	messageIDs := make([]types.MessageID, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			messageIDs = append(messageIDs, types.MessageID(id))
		}
	}
	return a.client.MarkRead(ctx, messageIDs, time.Now(), chat, sender)
}

func (a *whatsmeowAdapter) SendPresence(ctx context.Context, state string) error {
	switch state {
	case string(types.PresenceAvailable), string(types.PresenceUnavailable):
		return a.client.SendPresence(ctx, types.Presence(state))
	default:
		return fmt.Errorf("presence state must be %q or %q", types.PresenceAvailable, types.PresenceUnavailable)
	}
}

func (a *whatsmeowAdapter) SubscribePresence(ctx context.Context, jidString string) error {
	jid, err := a.resolveJID(ctx, jidString)
	if err != nil {
		return err
	}
	return a.client.SubscribePresence(ctx, jid)
}

func (a *whatsmeowAdapter) IsLoggedIn() bool {
	return a.client != nil && a.client.IsLoggedIn()
}

func (a *whatsmeowAdapter) IsConnected() bool {
	return a.client != nil && a.client.IsConnected()
}

func (a *whatsmeowAdapter) resolveJID(ctx context.Context, to string) (types.JID, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		return types.EmptyJID, errors.New("recipient is required")
	}
	if strings.Contains(to, "@") {
		jid, err := types.ParseJID(to)
		if err != nil {
			return types.EmptyJID, fmt.Errorf("parse recipient JID: %w", err)
		}
		if jid.Server == types.GroupServer {
			if jid.User == "" {
				return types.EmptyJID, errors.New("group JID is missing group id")
			}
			return jid, nil
		}
		if !isOneToOneUserJID(jid) || jid.User == "" {
			return types.EmptyJID, fmt.Errorf("recipient must be a 1:1 WhatsApp user JID or group JID")
		}
		if jid.Server == types.HiddenUserServer && a.client != nil && a.client.Store != nil && a.client.Store.LIDs != nil {
			pn, err := a.client.Store.LIDs.GetPNForLID(ctx, jid)
			if err == nil && !pn.IsEmpty() {
				return pn, nil
			}
		}
		return jid, nil
	}

	phone := normalizePhone(to)
	matches, err := a.client.IsOnWhatsApp(ctx, []string{phone})
	if err != nil {
		return types.EmptyJID, fmt.Errorf("check recipient registration: %w", err)
	}
	if len(matches) == 0 || !matches[0].IsIn {
		return types.EmptyJID, fmt.Errorf("recipient %q is not registered on WhatsApp", maskedPhone(phone))
	}
	return matches[0].JID, nil
}

func (a *whatsmeowAdapter) preferredSendJID(ctx context.Context, jid types.JID) types.JID {
	if jid.Server != types.DefaultUserServer || a.client == nil || a.client.Store == nil || a.client.Store.LIDs == nil {
		return jid
	}
	lid, err := a.client.Store.LIDs.GetLIDForPN(ctx, jid)
	if err == nil && !lid.IsEmpty() {
		return lid
	}
	return jid
}

func (a *whatsmeowAdapter) getContactsByMainJID(ctx context.Context) ([]contactInfo, error) {
	if a.rawDB == nil {
		return nil, nil
	}
	rows, err := a.rawDB.QueryContext(ctx, `
		SELECT their_jid, first_name, full_name, push_name, business_name
		FROM whatsmeow_contacts
	`)
	if err != nil {
		return nil, fmt.Errorf("query contacts fallback: %w", err)
	}
	defer rows.Close()

	var out []contactInfo
	for rows.Next() {
		var jidString string
		var first, full, push, business sql.NullString
		if err := rows.Scan(&jidString, &first, &full, &push, &business); err != nil {
			return nil, fmt.Errorf("scan contacts fallback: %w", err)
		}
		jid, err := types.ParseJID(jidString)
		if err != nil {
			continue
		}
		out = appendContact(ctx, out, a.preferredContactJID, jid, first.String, full.String, push.String, business.String)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts fallback: %w", err)
	}
	return out, nil
}

func appendContact(ctx context.Context, out []contactInfo, mapJID func(context.Context, types.JID) types.JID, jid types.JID, names ...string) []contactInfo {
	if !isOneToOneUserJID(jid) || jid.User == "" {
		return out
	}
	displayJID := mapJID(ctx, jid)
	name := firstNonEmpty(append(names, jid.User)...)
	return append(out, contactInfo{
		JID:  displayJID.String(),
		Name: name,
	})
}

func (a *whatsmeowAdapter) preferredContactJID(ctx context.Context, jid types.JID) types.JID {
	if jid.Server != types.HiddenUserServer || a.client == nil || a.client.Store == nil || a.client.Store.LIDs == nil {
		return jid
	}
	pn, err := a.client.Store.LIDs.GetPNForLID(ctx, jid)
	if err == nil && !pn.IsEmpty() {
		return pn
	}
	return jid
}

func isOneToOneUserJID(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer
}

func (a *whatsmeowAdapter) Disconnect() {
	a.client.Disconnect()
}

func (a *whatsmeowAdapter) Close() error {
	return a.container.Close()
}

func (a *whatsmeowAdapter) UserIDString() string {
	if a.client == nil || a.client.Store == nil || a.client.Store.ID == nil {
		return ""
	}
	return a.client.Store.ID.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
