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
	EventMessageReceived  = "message_received"
	EventContactsSynced   = "contacts_synced"
	EventManualReconnect  = "manual_login_reconnect"
	EventRiskStopped      = "risk_stopped"
)

const maxSendsPerRun = 5

const (
	freshLinkContactDelay = 2 * time.Minute
	freshLinkSendDelay    = 10 * time.Minute
	freshLinkMarkerFile   = "fresh-linked-at"
	riskMarkerFile        = "risk-stop.json"
)

var (
	activeOperationMinInterval = 5 * time.Second
	manualLoginReconnectDelay  = 2 * time.Second
	rateLimitStopDelay         = 30 * time.Minute
	riskStopDelay              = 24 * time.Hour
	sendTextTimeout            = 45 * time.Second
)

var allowedTestNumbers = map[string]struct{}{
	// Replace with the disposable test recipient in full country-code format.
	"15551234567": {},
}

// EventCallback is invoked from whatsmeow/event goroutines, not from the main
// goroutine. Android callers must forward across IPC and switch back to the UI
// thread before touching UI.
type EventCallback interface {
	OnEvent(eventType string, payloadJSON string)
}

type Client struct {
	callback   EventCallback
	dataDir    string
	deviceName string

	mu            sync.Mutex
	state         string
	started       bool
	wa            waAdapter
	hadSession    bool
	cancel        context.CancelFunc
	sendCount     int
	sentAt        map[string]time.Time
	freshLinkedAt time.Time
	nextActiveAt  time.Time
	riskUntil     time.Time
	riskReason    string

	trace traceRecorder
	newWA func(context.Context, string, string) (waAdapter, bool, error)
}

type waAdapter interface {
	AddEventHandler(func(any)) uint32
	GetQRChannel(context.Context) (<-chan qrItem, error)
	ConnectContext(context.Context) error
	ReconnectAfterLogin(context.Context) error
	SendText(context.Context, string, string, string) (sendTextResult, error)
	GetContacts(context.Context) ([]contactInfo, error)
	ResolveJID(context.Context, string) (string, error)
	Disconnect()
	Close() error
	UserIDString() string
}

type sendTextResult struct {
	ServerMessageID string
	RecipientJID    string
	RecipientServer string
	UsedLID         bool
}

type qrItem struct {
	Event   string
	Code    string
	Error   error
	Timeout time.Duration
}

type contactInfo struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

type persistedRiskStop struct {
	Until  string `json:"until"`
	Reason string `json:"reason"`
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
	freshLinkedAt := c.readFreshLinkedAt()
	riskUntil, riskReason := c.readRiskStop()
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
	c.freshLinkedAt = freshLinkedAt
	c.riskUntil = riskUntil
	c.riskReason = riskReason
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
		c.setState(StateDisconnected)
		c.enterRiskStopIfNeeded("connect", err)
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
	if _, ok := allowedTestNumbers[normalizedTo]; !ok {
		err := fmt.Errorf("recipient %q is not in the hard-coded test whitelist", maskedPhone(normalizedTo))
		c.emitMessageFailed(clientMsgId, "not_whitelisted", err.Error())
		return err
	}
	return c.sendTextChecked(normalizedTo, text, clientMsgId)
}

func (c *Client) SendText(to string, text string, clientMsgId string) (err error) {
	defer c.recoverAsError("SendText", &err)
	return c.sendTextChecked(strings.TrimSpace(to), text, clientMsgId)
}

func (c *Client) sendTextChecked(to string, text string, clientMsgId string) error {
	target := strings.TrimSpace(to)
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
	if target == "" {
		err := errors.New("recipient is required")
		c.emitMessageFailed(clientMsgId, "invalid_request", err.Error())
		return err
	}

	c.mu.Lock()
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		reason := c.riskReason
		c.mu.Unlock()
		err := riskStoppedError(reason, remaining)
		c.emitMessageFailed(clientMsgId, "risk_stopped", err.Error())
		return err
	}
	if c.sendCount >= maxSendsPerRun {
		c.mu.Unlock()
		err := fmt.Errorf("send limit exceeded: max %d messages per run", maxSendsPerRun)
		c.emitMessageFailed(clientMsgId, "send_limit_exceeded", err.Error())
		return err
	}
	if remaining := c.freshLinkRemainingLocked(freshLinkSendDelay); remaining > 0 {
		c.mu.Unlock()
		err := freshLinkCooldownError("sending", remaining)
		c.emitMessageFailed(clientMsgId, "fresh_link_cooldown", err.Error())
		return err
	}
	if remaining := c.activeOperationRemainingLocked(); remaining > 0 {
		c.mu.Unlock()
		err := activeOperationCooldownError("sending", remaining)
		c.emitMessageFailed(clientMsgId, "operation_backoff", err.Error())
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
	c.reserveActiveOperationLocked()
	c.mu.Unlock()
	if adapter == nil {
		err := errors.New("whatsmeow adapter is not initialized")
		c.emitMessageFailed(clientMsgId, "not_started", err.Error())
		return err
	}

	started := time.Now()
	startPayload := map[string]any{
		"clientMsgId": clientMsgId,
		"to_suffix":   recipientSuffix(target),
		"text_len":    len(text),
	}
	c.emit(EventMessageSendStart, startPayload, startPayload)

	ctx, cancel := context.WithTimeout(context.Background(), sendTextTimeout)
	defer cancel()
	result, err := adapter.SendText(ctx, target, text, clientMsgId)
	if err != nil {
		message := sanitizeError(err.Error(), target, normalizePhone(target), text)
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			c.emitMessageFailed(clientMsgId, "send_timeout", message, sendRoutePayload(result))
			return err
		}
		c.emitMessageFailed(clientMsgId, "send_failed", message, sendRoutePayload(result))
		c.enterRiskStopIfNeeded("sendText", err)
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
	for key, value := range sendRoutePayload(result) {
		payload[key] = value
	}
	c.emit(EventMessageSent, payload, payload)
	return nil
}

func (c *Client) GetContacts() (contactsJSON string, err error) {
	defer c.recoverAsError("GetContacts", &err)

	c.mu.Lock()
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		reason := c.riskReason
		c.mu.Unlock()
		return "", riskStoppedError(reason, remaining)
	}
	adapter := c.wa
	remaining := c.freshLinkRemainingLocked(freshLinkContactDelay)
	if remaining > 0 {
		c.mu.Unlock()
		return "", freshLinkCooldownError("reading contacts", remaining)
	}
	if remaining := c.activeOperationRemainingLocked(); remaining > 0 {
		c.mu.Unlock()
		return "", activeOperationCooldownError("reading contacts", remaining)
	}
	if c.state != StateConnected {
		state := c.state
		c.mu.Unlock()
		return "", fmt.Errorf("client state is %q, want %q", state, StateConnected)
	}
	c.reserveActiveOperationLocked()
	c.mu.Unlock()
	if adapter == nil {
		return "", errors.New("whatsmeow adapter is not initialized")
	}
	contacts, err := adapter.GetContacts(context.Background())
	if err != nil {
		return "", err
	}
	if contacts == nil {
		contacts = []contactInfo{}
	}
	b, err := json.Marshal(contacts)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) ResolveJID(to string) (jid string, err error) {
	defer c.recoverAsError("ResolveJID", &err)

	c.mu.Lock()
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		reason := c.riskReason
		c.mu.Unlock()
		return "", riskStoppedError(reason, remaining)
	}
	adapter := c.wa
	if remaining := c.activeOperationRemainingLocked(); remaining > 0 {
		c.mu.Unlock()
		return "", activeOperationCooldownError("resolving recipient", remaining)
	}
	if c.state != StateConnected {
		state := c.state
		c.mu.Unlock()
		return "", fmt.Errorf("client state is %q, want %q", state, StateConnected)
	}
	c.reserveActiveOperationLocked()
	c.mu.Unlock()
	if adapter == nil {
		return "", errors.New("whatsmeow adapter is not initialized")
	}
	return adapter.ResolveJID(context.Background(), strings.TrimSpace(to))
}

func (c *Client) SafetyStatus() (statusJSON string, err error) {
	defer c.recoverAsError("SafetyStatus", &err)
	return mustJSON(c.safetyStatus()), nil
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
	c.mu.Lock()
	c.freshLinkedAt = time.Time{}
	c.riskUntil = time.Time{}
	c.riskReason = ""
	c.nextActiveAt = time.Time{}
	c.mu.Unlock()
	return os.RemoveAll(c.dataDir)
}

func (c *Client) connectionParts() (waAdapter, bool, context.Context, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started || c.wa == nil || c.cancel == nil {
		return nil, false, nil, errors.New("client is not started")
	}
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		return nil, false, nil, riskStoppedError(c.riskReason, remaining)
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
		if err := c.markFreshLinkedDevice(time.Now()); err != nil {
			c.emitError("fresh_link_marker", err.Error())
		}
		c.emit(EventPaired, map[string]any{
			"jid_suffix": jidSuffix(v.ID.String()),
		}, map[string]any{
			"jid_suffix": jidSuffix(v.ID.String()),
		})
	case *events.ManualLoginReconnect:
		c.emit(EventManualReconnect, map[string]any{}, map[string]any{})
		go c.reconnectAfterManualLogin()
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
		c.enterRiskStopIfNeeded("connect_failure", errors.New(v.Reason.String()))
		c.emitError("connect_failure", v.Reason.String())
	case *events.TemporaryBan:
		c.enterRiskStop("temporary_ban", v.String(), riskStopDelay)
	case *events.Receipt:
		c.handleReceipt(v)
	case *events.Message:
		c.handleMessage(v)
	case *events.Contact, *events.PushName, *events.BusinessName:
		c.emit(EventContactsSynced, map[string]any{}, map[string]any{})
	}
}

func (c *Client) reconnectAfterManualLogin() {
	defer func() {
		if r := recover(); r != nil {
			c.emitError("manual_login_reconnect", fmt.Sprintf("panic: %v", r))
		}
	}()
	time.Sleep(manualLoginReconnectDelay)
	c.mu.Lock()
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		reason := c.riskReason
		c.mu.Unlock()
		c.emitError("manual_login_reconnect", riskStoppedError(reason, remaining).Error())
		return
	}
	adapter := c.wa
	cancel := c.cancel
	ctx, newCancel := context.WithCancel(context.Background())
	c.cancel = newCancel
	c.state = StateConnecting
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if adapter == nil {
		c.emitError("manual_login_reconnect", "whatsmeow adapter is not initialized")
		return
	}
	if err := adapter.ReconnectAfterLogin(ctx); err != nil {
		c.setState(StateDisconnected)
		c.enterRiskStopIfNeeded("manual_login_reconnect", err)
		c.emitError("manual_login_reconnect", err.Error())
	}
}

func (c *Client) handleMessage(message *events.Message) {
	if message == nil || message.Message == nil || message.Info.IsGroup || message.Info.IsFromMe {
		return
	}
	text := plainTextFromMessage(message)
	if strings.TrimSpace(text) == "" {
		return
	}
	fromJID := message.Info.Sender
	if fromJID.IsEmpty() {
		fromJID = message.Info.Chat
	}
	if fromJID.Server != "s.whatsapp.net" && fromJID.Server != "lid" {
		return
	}
	ts := message.Info.Timestamp.UTC().Format("2006-01-02T15:04:05.000Z")
	payload := map[string]any{
		"from_jid":      fromJID.String(),
		"from_suffix":   jidSuffix(fromJID.String()),
		"text":          text,
		"text_len":      len(text),
		"server_msg_id": string(message.Info.ID),
		"ts":            ts,
	}
	traceData := map[string]any{
		"from_suffix":   jidSuffix(fromJID.String()),
		"text_len":      len(text),
		"server_msg_id": string(message.Info.ID),
		"ts":            ts,
	}
	c.emit(EventMessageReceived, payload, traceData)
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
	c.callback.OnEvent(eventType, payloadJSON)
}

func (c *Client) freshLinkRemainingLocked(delay time.Duration) time.Duration {
	if c.freshLinkedAt.IsZero() {
		return 0
	}
	until := c.freshLinkedAt.Add(delay)
	if remaining := time.Until(until); remaining > 0 {
		return remaining
	}
	return 0
}

func (c *Client) activeOperationRemainingLocked() time.Duration {
	if c.nextActiveAt.IsZero() {
		return 0
	}
	if remaining := time.Until(c.nextActiveAt); remaining > 0 {
		return remaining
	}
	return 0
}

func (c *Client) reserveActiveOperationLocked() {
	c.nextActiveAt = time.Now().Add(activeOperationMinInterval)
}

func (c *Client) riskRemainingLocked() time.Duration {
	if c.riskUntil.IsZero() {
		return 0
	}
	if remaining := time.Until(c.riskUntil); remaining > 0 {
		return remaining
	}
	return 0
}

func (c *Client) markFreshLinkedDevice(at time.Time) error {
	c.mu.Lock()
	c.freshLinkedAt = at
	c.mu.Unlock()
	return os.WriteFile(c.freshLinkMarkerPath(), []byte(at.UTC().Format(time.RFC3339Nano)), 0o600)
}

func (c *Client) readFreshLinkedAt() time.Time {
	b, err := os.ReadFile(c.freshLinkMarkerPath())
	if err != nil {
		return time.Time{}
	}
	at, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(b)))
	if err != nil {
		return time.Time{}
	}
	return at
}

func (c *Client) freshLinkMarkerPath() string {
	return filepath.Join(c.dataDir, freshLinkMarkerFile)
}

func (c *Client) enterRiskStopIfNeeded(where string, err error) {
	if err == nil {
		return
	}
	if duration := riskStopDuration(err.Error()); duration > 0 {
		c.enterRiskStop(where, err.Error(), duration)
	}
}

func (c *Client) enterRiskStop(where string, reason string, duration time.Duration) {
	until := time.Now().Add(duration)
	safeReason := sanitizeTraceString(reason)

	c.mu.Lock()
	if !c.riskUntil.IsZero() && c.riskUntil.After(until) {
		until = c.riskUntil
	}
	c.riskUntil = until
	c.riskReason = safeReason
	cancel := c.cancel
	adapter := c.wa
	c.state = StateDisconnected
	c.mu.Unlock()

	_ = c.writeRiskStop(until, safeReason)
	if cancel != nil {
		cancel()
	}
	if adapter != nil {
		adapter.Disconnect()
	}
	payload := map[string]any{
		"where":               where,
		"reason":              safeReason,
		"retry_after_seconds": int(time.Until(until).Seconds()),
	}
	c.emit(EventRiskStopped, payload, payload)
}

func riskStopDuration(message string) time.Duration {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "463"),
		strings.Contains(lower, "temporary ban"),
		strings.Contains(lower, "temporarily banned"),
		strings.Contains(lower, "temp banned"):
		return riskStopDelay
	case strings.Contains(lower, "rate-overlimit"),
		strings.Contains(lower, "rate limited"),
		strings.Contains(lower, "rate limit"),
		strings.Contains(lower, "429"):
		return rateLimitStopDelay
	default:
		return 0
	}
}

func (c *Client) readRiskStop() (time.Time, string) {
	b, err := os.ReadFile(c.riskMarkerPath())
	if err != nil {
		return time.Time{}, ""
	}
	var persisted persistedRiskStop
	if err := json.Unmarshal(b, &persisted); err != nil {
		return time.Time{}, ""
	}
	until, err := time.Parse(time.RFC3339Nano, persisted.Until)
	if err != nil || time.Until(until) <= 0 {
		return time.Time{}, ""
	}
	return until, persisted.Reason
}

func (c *Client) writeRiskStop(until time.Time, reason string) error {
	b, err := json.Marshal(persistedRiskStop{
		Until:  until.UTC().Format(time.RFC3339Nano),
		Reason: reason,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(c.riskMarkerPath(), b, 0o600)
}

func (c *Client) riskMarkerPath() string {
	return filepath.Join(c.dataDir, riskMarkerFile)
}

func (c *Client) safetyStatus() map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := map[string]any{
		"state":                              c.state,
		"risk_stopped":                       false,
		"risk_retry_after_seconds":           0,
		"fresh_contacts_retry_after_seconds": 0,
		"fresh_send_retry_after_seconds":     0,
		"operation_retry_after_seconds":      0,
	}
	if remaining := c.riskRemainingLocked(); remaining > 0 {
		status["risk_stopped"] = true
		status["risk_reason"] = c.riskReason
		status["risk_retry_after_seconds"] = int(remaining.Seconds())
	}
	if remaining := c.freshLinkRemainingLocked(freshLinkContactDelay); remaining > 0 {
		status["fresh_contacts_retry_after_seconds"] = int(remaining.Seconds())
	}
	if remaining := c.freshLinkRemainingLocked(freshLinkSendDelay); remaining > 0 {
		status["fresh_send_retry_after_seconds"] = int(remaining.Seconds())
	}
	if remaining := c.activeOperationRemainingLocked(); remaining > 0 {
		status["operation_retry_after_seconds"] = int(remaining.Seconds())
	}
	return status
}

func freshLinkCooldownError(action string, remaining time.Duration) error {
	remaining = remaining.Round(time.Second)
	if remaining < time.Second {
		remaining = time.Second
	}
	return fmt.Errorf("fresh linked device cooldown: wait %s before %s", remaining, action)
}

func activeOperationCooldownError(action string, remaining time.Duration) error {
	remaining = remaining.Round(time.Second)
	if remaining < time.Second {
		remaining = time.Second
	}
	return fmt.Errorf("operation backoff: wait %s before %s", remaining, action)
}

func riskStoppedError(reason string, remaining time.Duration) error {
	remaining = remaining.Round(time.Second)
	if remaining < time.Second {
		remaining = time.Second
	}
	if reason == "" {
		reason = "risk stop is active"
	}
	return fmt.Errorf("risk stop active: %s; retry after %s", reason, remaining)
}

func plainTextFromMessage(message *events.Message) string {
	if message == nil || message.Message == nil {
		return ""
	}
	if text := message.Message.GetConversation(); text != "" {
		return text
	}
	if extended := message.Message.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	return ""
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

func (c *Client) emitMessageFailed(clientMsgId string, errorCode string, message string, extras ...map[string]any) {
	payload := map[string]any{
		"clientMsgId": clientMsgId,
		"error_code":  errorCode,
		"error":       message,
	}
	for _, extra := range extras {
		for key, value := range extra {
			payload[key] = value
		}
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

func recipientSuffix(recipient string) string {
	if strings.Contains(recipient, "@") {
		return jidSuffix(recipient)
	}
	return maskedPhone(normalizePhone(recipient))
}

func sendRoutePayload(result sendTextResult) map[string]any {
	payload := make(map[string]any)
	if result.RecipientJID != "" {
		payload["recipient_suffix"] = jidSuffix(result.RecipientJID)
	}
	if result.RecipientServer != "" {
		payload["recipient_server"] = result.RecipientServer
	}
	if result.RecipientJID != "" || result.RecipientServer != "" {
		payload["used_lid"] = result.UsedLID
	}
	return payload
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
