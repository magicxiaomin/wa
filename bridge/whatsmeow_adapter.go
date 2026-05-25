package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

type whatsmeowAdapter struct {
	client    *whatsmeow.Client
	container *sqlstore.Container
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

	return &whatsmeowAdapter{
		client:    whatsmeow.NewClient(deviceStore, waLog.Noop),
		container: container,
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

func (a *whatsmeowAdapter) SendText(ctx context.Context, phone string, text string, clientMsgId string) (sendTextResult, error) {
	matches, err := a.client.IsOnWhatsApp(ctx, []string{phone})
	if err != nil {
		return sendTextResult{}, fmt.Errorf("check recipient registration: %w", err)
	}
	if len(matches) == 0 || !matches[0].IsIn {
		return sendTextResult{}, fmt.Errorf("recipient %q is not registered on WhatsApp", maskedPhone(phone))
	}

	resp, err := a.client.SendMessage(ctx, matches[0].JID, &waE2E.Message{
		Conversation: proto.String(text),
	})
	if err != nil {
		return sendTextResult{}, err
	}
	return sendTextResult{
		ServerMessageID: string(resp.ID),
		RecipientJID:    matches[0].JID.String(),
	}, nil
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
