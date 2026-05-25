package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types/events"
)

const (
	StateInitializing = "initializing"
	StateWaitingQR    = "waiting_qr"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateDisconnected = "disconnected"
	StateLoggedOut    = "logged_out"
)

const (
	EventBridgeStarted    = "bridge_started"
	EventQRGenerated      = "qr_generated"
	EventPaired           = "paired"
	EventConnecting       = "connecting"
	EventConnected        = "connected"
	EventDisconnected     = "disconnected"
	EventSessionRestored  = "session_restored"
	EventSessionInvalid   = "session_invalid"
	EventError            = "error"
	EventMessageSendStart = "message_send_start"
	EventMessageSent      = "message_sent"
	EventMessageAck       = "message_ack"
	EventMessageFailed    = "message_failed"
)

const maxSendsPerRun = 5

var allowedTestNumbers = map[string]struct{}{
	// Replace with the disposable test recipient in full country-code format.
	"15551234567": {},
}

// EventCallback is invoked from whatsmeow/event goroutines, not from the main
// goroutine. Android callers must switch back to the UI thread before touching UI.
type EventCallback func(eventType string, payloadJSON string)

type Client struct {
	callback   EventCallback
	dataDir    string
	deviceName string

	mu         sync.Mutex
	state      string
	started    bool
	wa         waAdapter
	hadSession bool
	cancel     context.CancelFunc
	sendCount  int
	sentAt     map[string]time.Time

	trace traceRecorder
	newWA func(context.Context, string, string) (waAdapter, bool, error)
}

type waAdapter interface {
	AddEventHandler(func(any)) uint32
	GetQRChannel(context.Context) (<-chan qrItem, error)
	ConnectContext(context.Context) error
	SendText(context.Context, string, string, string) (sendTextResult, error)
	Disconnect()
	Close() error
	UserIDString() string
}

type sendTextResult struct {
	ServerMessageID string
	RecipientJID    string
}

type qrItem struct {
	Event   string
	Code    string
	Error   error
	Timeout time.Duration
}

func NewClient(callback EventCallback, dataDir string, deviceName string) (client *Client, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic in NewClient: %v", r)
		}
	}()

	if dataDir == "" {
		return nil, errors.New("dataDir is required")
	}
	if deviceName == "" {
		deviceName = "wa-desktop-poc"
	}
	c := &Client{
		callback:   callback,
		dataDir:    dataDir,
		deviceName: deviceName,
		state:      StateInitializing,
		sentAt:     make(map[string]time.Time),
	}
	c.newWA = newWhatsmeowAdapter
	return c, nil
}

func (c *Client) Start() (err error) {
	defer c.recoverAsError("Start", &err)

	if err := os.MkdirAll(c.dataDir, 0o700); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	adapter, hadSession, err := c.newWA(ctx, c.dataDir, c.deviceName)
	if err != nil {
		cancel()
		return err
	}
	adapter.AddEventHandler(c.handleWAEvent)

	c.mu.Lock()
	c.wa = adapter
	c.hadSession = hadSession
	c.cancel = cancel
	c.started = true
	c.state = StateDisconnected
	c.mu.Unlock()

	c.emit(EventBridgeStarted, map[string]any{
		"data_dir": filepath.Clean(c.dataDir),
	}, map[string]any{})
	return nil
}

func (c *Client) Stop() (err error) {
	defer c.recoverAsError("Stop", &err)

	c.mu.Lock()
	cancel := c.cancel
	adapter := c.wa
	c.cancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if adapter != nil {
		adapter.Disconnect()
		if closeErr := adapter.Close(); closeErr != nil {
			return closeErr
		}
	}
	c.setState(StateDisconnected)
	return nil
}

func (c *Client) Connect() (err error) {
	defer c.recoverAsError("Connect", &err)

	adapter, hadSession, ctx, err := c.connectionParts()
	if err != nil {
		return err
	}
	c.setState(StateConnecting)
	c.emit(EventConnecting, map[string]any{}, map[string]any{})

	if !hadSession {
		qrChan, err := adapter.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("get QR channel before connect: %w", err)
		}
		go c.consumeQR(qrChan)
	}

	if err := adapter.ConnectContext(ctx); err != nil {
		return err
	}
	return nil
}

func (c *Client) Disconnect() (err error) {
	defer c.recoverAsError("Disconnect", &err)

	c.mu.Lock()
	cancel := c.cancel
	adapter := c.wa
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if adapter != nil {
		adapter.Disconnect()
	}
	c.setState(StateDisconnected)
	c.emit(EventDisconnected, map[string]any{
		"reason":         "manual_disconnect",
		"will_reconnect": false,
	}, map[string]any{
		"reason":         "manual_disconnect",
		"will_reconnect": false,
	})
	return nil
}

func (c *Client) RequestPairing() (err error) {
	defer c.recoverAsError("RequestPairing", &err)
	return errors.New("pairing code login is not implemented in Step 1; use QR login")
}

func (c *Client) GetState() (state string) {
	defer func() {
		if r := recover(); r != nil {
			c.emitError("GetState", fmt.Sprintf("panic: %v", r))
			state = StateDisconnected
		}
	}()

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) SendTextForTest(to string, text string, clientMsgId string) (err error) {
	defer c.recoverAsError("SendTextForTest", &err)

	normalizedTo := normalizePhone(to)
	if clientMsgId == "" {
		err := errors.New("clientMsgId is required")
		c.emitMessageFailed(clientMsgId, "invalid_request", err.Error())
		return err
	}
	if strings.TrimSpace(text) == "" {
		err := errors.New("text is required")
		c.emitMessageFailed(clientMsgId, "invalid_request", err.Error())
		return err
	}
	if _, ok := allowedTestNumbers[normalizedTo]; !ok {
		err := fmt.Errorf("recipient %q is not in the hard-coded test whitelist", maskedPhone(normalizedTo))
		c.emitMessageFailed(clientMsgId, "not_whitelisted", err.Error())
		return err
	}

	c.mu.Lock()
	if c.sendCount >= maxSendsPerRun {
		c.mu.Unlock()
		err := fmt.Errorf("send limit exceeded: max %d messages per run", maxSendsPerRun)
		c.emitMessageFailed(clientMsgId, "send_limit_exceeded", err.Error())
		return err
	}
	if c.state != StateConnected {
		c.mu.Unlock()
		err := fmt.Errorf("client state is %q, want %q", c.state, StateConnected)
		c.emitMessageFailed(clientMsgId, "not_connected", err.Error())
		return err
	}
	adapter := c.wa
	c.sendCount++
	c.mu.Unlock()
	if adapter == nil {
		err := errors.New("whatsmeow adapter is not initialized")
		c.emitMessageFailed(clientMsgId, "not_started", err.Error())
		return err
	}

	started := time.Now()
	startPayload := map[string]any{
		"clientMsgId": clientMsgId,
		"to_suffix":   maskedPhone(normalizedTo),
		"text_len":    len(text),
	}
	c.emit(EventMessageSendStart, startPayload, startPayload)

	result, err := adapter.SendText(context.Background(), normalizedTo, text, clientMsgId)
	if err != nil {
		message := sanitizeError(err.Error(), normalizedTo, text)
		c.emitMessageFailed(clientMsgId, "send_failed", message)
		return err
	}

	latencyMS := time.Since(started).Milliseconds()
	c.mu.Lock()
	c.sentAt[result.ServerMessageID] = started
	c.mu.Unlock()
	payload := map[string]any{
		"clientMsgId":   clientMsgId,
		"server_msg_id": result.ServerMessageID,
		"latency_ms":    latencyMS,
	}
	c.emit(EventMessageSent, payload, payload)
	return nil
}

func (c *Client) ExportTrace(path string) (err error) {
	defer c.recoverAsError("ExportTrace", &err)
	return c.trace.export(path)
}

func (c *Client) ClearSession() (err error) {
	defer c.recoverAsError("ClearSession", &err)
	if err := c.Stop(); err != nil {
		return err
	}
	return os.RemoveAll(c.dataDir)
}

func (c *Client) connectionParts() (waAdapter, bool, context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.wa == nil || c.cancel == nil {
		return nil, false, nil, errors.New("client is not started")
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	return c.wa, c.hadSession, ctx, nil
}

func (c *Client) consumeQR(qrChan <-chan qrItem) {
	defer func() {
		if r := recover(); r != nil {
			c.emitError("consumeQR", fmt.Sprintf("panic: %v", r))
		}
	}()

	for item := range qrChan {
		switch item.Event {
		case "code":
			c.setState(StateWaitingQR)
			c.emit(EventQRGenerated, map[string]any{
				"qr":              item.Code,
				"qr_len":          len(item.Code),
				"timeout_seconds": int(item.Timeout.Seconds()),
			}, map[string]any{
				"qr_len": len(item.Code),
			})
		case "success":
			c.emit(EventPaired, map[string]any{}, map[string]any{})
		case "error":
			message := "unknown QR error"
			if item.Error != nil {
				message = item.Error.Error()
			}
			c.emitError("qr_channel", message)
		default:
			c.emit("qr_"+item.Event, map[string]any{}, map[string]any{})
		}
	}
}

func (c *Client) handleWAEvent(evt any) {
	defer func() {
		if r := recover(); r != nil {
			c.emitError("handleWAEvent", fmt.Sprintf("panic: %v", r))
		}
	}()

	switch v := evt.(type) {
	case *events.PairSuccess:
		c.emit(EventPaired, map[string]any{
			"jid_suffix": jidSuffix(v.ID.String()),
		}, map[string]any{
			"jid_suffix": jidSuffix(v.ID.String()),
		})
	case *events.Connected:
		c.setState(StateConnected)
		if c.wasSessionRestore() {
			c.emit(EventSessionRestored, map[string]any{
				"jid_suffix": jidSuffix(c.userIDString()),
			}, map[string]any{
				"jid_suffix": jidSuffix(c.userIDString()),
			})
		}
		c.emit(EventConnected, map[string]any{
			"jid_suffix": jidSuffix(c.userIDString()),
		}, map[string]any{
			"jid_suffix": jidSuffix(c.userIDString()),
		})
	case *events.Disconnected:
		c.setState(StateDisconnected)
		c.emit(EventDisconnected, map[string]any{
			"reason":         "websocket_disconnected",
			"will_reconnect": true,
		}, map[string]any{
			"reason":         "websocket_disconnected",
			"will_reconnect": true,
		})
	case *events.LoggedOut:
		c.setState(StateLoggedOut)
		c.emit(EventSessionInvalid, map[string]any{
			"reason": v.Reason.String(),
		}, map[string]any{
			"reason": v.Reason.String(),
		})
	case *events.ConnectFailure:
		if v.Reason.IsLoggedOut() {
			c.setState(StateLoggedOut)
			c.emit(EventSessionInvalid, map[string]any{
				"reason": v.Reason.String(),
			}, map[string]any{
				"reason": v.Reason.String(),
			})
			return
		}
		c.emitError("connect_failure", v.Reason.String())
	case *events.Receipt:
		c.handleReceipt(v)
	}
}

func (c *Client) handleReceipt(receipt *events.Receipt) {
	for _, id := range receipt.MessageIDs {
		serverID := string(id)
		c.mu.Lock()
		started, ok := c.sentAt[serverID]
		c.mu.Unlock()
		if !ok {
			continue
		}
		payload := map[string]any{
			"server_msg_id": serverID,
			"ack_level":     ackLevel(string(receipt.Type)),
			"latency_ms":    time.Since(started).Milliseconds(),
		}
		c.emit(EventMessageAck, payload, payload)
	}
}

func (c *Client) setState(state string) {
	c.mu.Lock()
	c.state = state
	c.mu.Unlock()
}

func (c *Client) emit(eventType string, payload any, traceData any) {
	payloadJSON := mustJSON(payload)
	c.trace.add(eventType, c.GetState(), traceData)
	if c.callback == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			c.trace.add(EventError, c.GetState(), map[string]any{
				"where":   "callback",
				"message": fmt.Sprintf("panic: %v", r),
			})
		}
	}()
	c.callback(eventType, payloadJSON)
}

func (c *Client) emitError(where string, message string) {
	c.emit(EventError, map[string]any{
		"where":   where,
		"message": message,
	}, map[string]any{
		"where":   where,
		"message": message,
	})
}

func (c *Client) emitMessageFailed(clientMsgId string, errorCode string, message string) {
	payload := map[string]any{
		"clientMsgId": clientMsgId,
		"error_code":  errorCode,
		"error":       message,
	}
	c.emit(EventMessageFailed, payload, payload)
}

func (c *Client) recoverAsError(where string, errp *error) {
	if r := recover(); r != nil {
		err := fmt.Errorf("panic in %s: %v", where, r)
		c.emitError(where, err.Error())
		*errp = err
	}
}

func (c *Client) wasSessionRestore() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hadSession
}

func (c *Client) userIDString() string {
	c.mu.Lock()
	adapter := c.wa
	c.mu.Unlock()
	if adapter == nil {
		return ""
	}
	return adapter.UserIDString()
}

func mustJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `{"marshal_error":"payload could not be encoded"}`
	}
	return string(b)
}

func jidSuffix(jid string) string {
	if jid == "" {
		return ""
	}
	at := strings.IndexByte(jid, '@')
	number := jid
	if at >= 0 {
		number = jid[:at]
	}
	if deviceSep := strings.IndexByte(number, ':'); deviceSep >= 0 {
		number = number[:deviceSep]
	}
	if len(number) <= 4 {
		return "..." + number
	}
	return "..." + number[len(number)-4:]
}

func normalizePhone(phone string) string {
	replacer := strings.NewReplacer("+", "", " ", "", "-", "", "(", "", ")", "")
	return replacer.Replace(strings.TrimSpace(phone))
}

func maskedPhone(phone string) string {
	if len(phone) <= 4 {
		return "..." + phone
	}
	return "..." + phone[len(phone)-4:]
}

func sanitizeError(message string, sensitive ...string) string {
	out := message
	for _, value := range sensitive {
		if value == "" {
			continue
		}
		out = strings.ReplaceAll(out, value, "[redacted]")
	}
	return out
}

func ackLevel(receiptType string) int {
	switch receiptType {
	case "read", "read-self":
		return 2
	case "played", "played-self":
		return 3
	default:
		return 1
	}
}
