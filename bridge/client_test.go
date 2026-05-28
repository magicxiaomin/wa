package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	waevents "go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestStartAndConnectMountHandlersAndQRBeforeNetworkConnect(t *testing.T) {
	eventsRec := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(eventsRec, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, false, nil
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !eventsRec.seen("bridge_started") {
		t.Fatalf("Start() did not emit bridge_started; events=%v", eventsRec.types())
	}

	if err := c.Connect(); err != nil {
		t.Fatalf("Connect() error = %v", err)
	}

	wantOrder := []string{"add_event_handler", "get_qr_channel", "connect"}
	if got := strings.Join(fake.calls, ","); got != strings.Join(wantOrder, ",") {
		t.Fatalf("wrong whatsmeow setup order: got %v want %v", fake.calls, wantOrder)
	}

	const qrCode = "2@test-qr-code"
	fake.qr <- qrItem{Event: "code", Code: qrCode}

	got := eventsRec.waitFor(t, "qr_generated")
	var payload map[string]any
	if err := json.Unmarshal([]byte(got.payload), &payload); err != nil {
		t.Fatalf("qr payload is not JSON: %v", err)
	}
	if payload["qr"] != qrCode {
		t.Fatalf("qr payload missing raw QR code for terminal display: %#v", payload)
	}
	if c.GetState() != StateWaitingQR {
		t.Fatalf("state after QR = %q, want %q", c.GetState(), StateWaitingQR)
	}
}

func TestConnectRecoversPanicAsErrorEvent(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.panicOnConnect = true

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, false, nil
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := c.Connect(); err == nil {
		t.Fatal("Connect() error = nil, want panic converted to error")
	}
	if !events.seen("error") {
		t.Fatalf("panic was not emitted as error event; events=%v", events.types())
	}
}

func TestManualLoginReconnectReconnectsAfterPair(t *testing.T) {
	oldDelay := manualLoginReconnectDelay
	manualLoginReconnectDelay = 0
	defer func() { manualLoginReconnectDelay = oldDelay }()

	eventsRec := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(eventsRec, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, false, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fake.handler(&waevents.ManualLoginReconnect{})
	eventsRec.waitFor(t, EventManualReconnect)
	deadline := time.After(2 * time.Second)
	for {
		if fake.reconnectCalls > 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("manual login reconnect did not reach adapter; calls=%v", fake.calls)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestStartInitializesModerncSQLiteSessionStore(t *testing.T) {
	events := newEventRecorder()
	dataDir := t.TempDir()

	c, err := NewClient(events, dataDir, "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := c.Stop(); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	}()

	if !events.seen("bridge_started") {
		t.Fatalf("Start() did not emit bridge_started; events=%v", events.types())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "whatsmeow.db")); err != nil {
		t.Fatalf("session db was not created: %v", err)
	}
}

func TestClearSessionResetsClientState(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	dataDir := t.TempDir()

	c, err := NewClient(events, dataDir, "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)
	c.mu.Lock()
	c.sentAt["server-1"] = time.Now()
	c.freshLinkedAt = time.Now()
	c.riskUntil = time.Now().Add(time.Hour)
	c.riskReason = "test"
	c.nextActiveAt = time.Now().Add(time.Minute)
	c.mu.Unlock()

	if err := c.ClearSession(); err != nil {
		t.Fatalf("ClearSession() error = %v", err)
	}

	c.mu.Lock()
	if c.wa != nil || c.started || c.hadSession || c.cancel != nil {
		t.Fatalf("client state not reset: wa=%v started=%v hadSession=%v cancelNil=%v", c.wa, c.started, c.hadSession, c.cancel == nil)
	}
	if len(c.sentAt) != 0 {
		t.Fatalf("sentAt len = %d, want 0", len(c.sentAt))
	}
	if !c.freshLinkedAt.IsZero() || !c.riskUntil.IsZero() || c.riskReason != "" || !c.nextActiveAt.IsZero() {
		t.Fatalf("safety state not reset: fresh=%v risk=%v reason=%q next=%v", c.freshLinkedAt, c.riskUntil, c.riskReason, c.nextActiveAt)
	}
	if c.state != StateDisconnected {
		t.Fatalf("state = %q, want %q", c.state, StateDisconnected)
	}
	c.mu.Unlock()
	if !fake.disconnected || !fake.closed {
		t.Fatalf("adapter cleanup incomplete: disconnected=%v closed=%v", fake.disconnected, fake.closed)
	}
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("session data dir still exists or unexpected stat error: %v", err)
	}
	if err := c.Connect(); err == nil || !strings.Contains(err.Error(), "client is not started") {
		t.Fatalf("Connect() error = %v, want client is not started", err)
	}
}

func TestJIDSuffixStripsDeviceIDBeforeMasking(t *testing.T) {
	got := jidSuffix("1234567892:1@s.whatsapp.net")
	if got != "...7892" {
		t.Fatalf("jidSuffix() = %q, want %q", got, "...7892")
	}
}

func TestSendTextForTestEmitsSentEventsAndRedactedTrace(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResult = sendTextResult{
		ServerMessageID: "3EB0SERVERMSGID",
		RecipientJID:    "15551234567@s.whatsapp.net",
	}

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	const secretText = "this message body must never enter trace"
	if err := c.SendTextForTest("15551234567", secretText, "client-1"); err != nil {
		t.Fatalf("SendTextForTest() error = %v", err)
	}

	start := events.waitFor(t, EventMessageSendStart)
	sent := events.waitFor(t, EventMessageSent)
	if strings.Contains(start.payload, secretText) || strings.Contains(sent.payload, secretText) {
		t.Fatalf("send event leaked message text: start=%s sent=%s", start.payload, sent.payload)
	}
	if strings.Contains(start.payload, "15551234567") || strings.Contains(sent.payload, "15551234567") {
		t.Fatalf("send event leaked full phone: start=%s sent=%s", start.payload, sent.payload)
	}
	if !strings.Contains(sent.payload, "3EB0SERVERMSGID") {
		t.Fatalf("message_sent missing server id: %s", sent.payload)
	}

	tracePath := filepath.Join(t.TempDir(), "trace.json")
	if err := c.ExportTrace(tracePath); err != nil {
		t.Fatalf("ExportTrace() error = %v", err)
	}
	traceBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	trace := string(traceBytes)
	if strings.Contains(trace, secretText) {
		t.Fatalf("trace leaked message text: %s", trace)
	}
	if strings.Contains(trace, "15551234567") {
		t.Fatalf("trace leaked full phone: %s", trace)
	}
}

func TestSendTextAllowsSingleGroupJID(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResultsByTarget = map[string]sendTextResult{
		"120363000000000000@g.us": {
			ServerMessageID: "group-server-1",
			RecipientJID:    "120363000000000000@g.us",
			RecipientServer: "g.us",
		},
	}

	c := newStartedConnectedTestClient(t, events, fake)

	if err := c.SendText("120363000000000000@g.us", "hello group", "group-client-1"); err != nil {
		t.Fatalf("SendText() error = %v", err)
	}
	if fake.sendCalls != 1 {
		t.Fatalf("SendText() calls = %d, want 1", fake.sendCalls)
	}
	if fake.sendTargets[0] != "120363000000000000@g.us" {
		t.Fatalf("SendText() target = %q", fake.sendTargets[0])
	}
	sent := events.waitFor(t, EventMessageSent)
	if !strings.Contains(sent.payload, `"server_msg_id":"group-server-1"`) {
		t.Fatalf("message_sent missing server_msg_id: %s", sent.payload)
	}
	if !strings.Contains(sent.payload, `"recipient_server":"g.us"`) {
		t.Fatalf("message_sent missing recipient_server: %s", sent.payload)
	}
}

func TestWhatsmeowAdapterResolveJIDAllowsGroupJID(t *testing.T) {
	adapter := &whatsmeowAdapter{}

	jid, err := adapter.resolveJID(context.Background(), "120363000000000000@g.us")
	if err != nil {
		t.Fatalf("resolveJID() error = %v", err)
	}
	if jid.String() != "120363000000000000@g.us" {
		t.Fatalf("resolveJID() = %q", jid.String())
	}
}

func TestSendTextTimeoutEmitsFailedWithoutRiskStop(t *testing.T) {
	oldTimeout := sendTextTimeout
	oldInterval := activeOperationMinInterval
	sendTextTimeout = 10 * time.Millisecond
	activeOperationMinInterval = 0
	defer func() {
		sendTextTimeout = oldTimeout
		activeOperationMinInterval = oldInterval
	}()

	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.waitForSendContext = true

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	if err := c.SendText("15551234567@s.whatsapp.net", "hello", "client-timeout"); err == nil {
		t.Fatal("SendText() error = nil, want timeout")
	}
	got := events.waitFor(t, EventMessageFailed)
	if !strings.Contains(got.payload, `"error_code":"send_timeout"`) {
		t.Fatalf("message_failed missing send_timeout: %s", got.payload)
	}
	if events.seen(EventRiskStopped) {
		t.Fatalf("send timeout should not enter risk stop; events=%v", events.types())
	}
}

func TestSendTextForTestRejectsNonWhitelistedRecipient(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	if err := c.SendTextForTest("19999999999", "hello", "client-denied"); err == nil {
		t.Fatal("SendTextForTest() error = nil, want whitelist rejection")
	}
	if fake.sendCalls != 0 {
		t.Fatalf("non-whitelisted send reached adapter %d times", fake.sendCalls)
	}
	if !events.seen(EventMessageFailed) {
		t.Fatalf("non-whitelisted send did not emit message_failed; events=%v", events.types())
	}
}

func TestSendTextForTestRejectsAfterSendLimit(t *testing.T) {
	oldInterval := activeOperationMinInterval
	activeOperationMinInterval = 0
	defer func() { activeOperationMinInterval = oldInterval }()

	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResult = sendTextResult{ServerMessageID: "3EB0OK", RecipientJID: "15551234567@s.whatsapp.net"}

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	for i := 0; i < maxSendsPerRun; i++ {
		if err := c.SendTextForTest("15551234567", "hello", "client-ok"); err != nil {
			t.Fatalf("SendTextForTest() #%d error = %v", i+1, err)
		}
	}
	if err := c.SendTextForTest("15551234567", "hello", "client-limit"); err == nil {
		t.Fatal("SendTextForTest() after limit error = nil")
	}
	if fake.sendCalls != maxSendsPerRun {
		t.Fatalf("adapter send calls = %d, want %d", fake.sendCalls, maxSendsPerRun)
	}
}

func TestSendTextMultiSendsTwoAndThreeRecipients(t *testing.T) {
	restore := overrideMultiSendTestTiming()
	defer restore()

	for _, tc := range []struct {
		name    string
		targets []string
	}{
		{
			name:    "two",
			targets: []string{"15550000001@s.whatsapp.net", "15550000002@s.whatsapp.net"},
		},
		{
			name:    "three",
			targets: []string{"15550000001@s.whatsapp.net", "15550000002@s.whatsapp.net", "15550000003@s.whatsapp.net"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			events := newEventRecorder()
			fake := newFakeWAAdapter()
			fake.sendResultsByTarget = map[string]sendTextResult{
				"15550000001@s.whatsapp.net": {ServerMessageID: "server-1", RecipientJID: "15550000001@s.whatsapp.net"},
				"15550000002@s.whatsapp.net": {ServerMessageID: "server-2", RecipientJID: "15550000002@s.whatsapp.net"},
				"15550000003@s.whatsapp.net": {ServerMessageID: "server-3", RecipientJID: "15550000003@s.whatsapp.net"},
			}
			c := newStartedConnectedTestClient(t, events, fake)

			payload, err := json.Marshal(tc.targets)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			got, err := c.SendTextMulti(string(payload), "hello multi", "multi-client")
			if err != nil {
				t.Fatalf("SendTextMulti() error = %v", err)
			}

			var results []multiSendResult
			if err := json.Unmarshal([]byte(got), &results); err != nil {
				t.Fatalf("results JSON invalid: %v; %s", err, got)
			}
			if len(results) != len(tc.targets) {
				t.Fatalf("results len = %d, want %d: %s", len(results), len(tc.targets), got)
			}
			if fake.sendCalls != len(tc.targets) {
				t.Fatalf("send calls = %d, want %d", fake.sendCalls, len(tc.targets))
			}
			for i, result := range results {
				if !result.OK {
					t.Fatalf("result %d not ok: %#v", i, result)
				}
				if result.ServerMessageID == "" {
					t.Fatalf("result %d missing server_msg_id: %#v", i, result)
				}
				if result.JIDSuffix == "" || strings.Contains(result.JIDSuffix, "@") {
					t.Fatalf("result %d has unsafe jid_suffix: %#v", i, result)
				}
			}
		})
	}
}

func TestSendTextMultiRejectsFourRecipientsWithoutSending(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	c := newStartedConnectedTestClient(t, events, fake)

	_, err := c.SendTextMulti(`[
		"15550000001@s.whatsapp.net",
		"15550000002@s.whatsapp.net",
		"15550000003@s.whatsapp.net",
		"15550000004@s.whatsapp.net"
	]`, "hello", "multi-limit")
	if err == nil || !strings.Contains(err.Error(), "exceeds max 3 recipients") {
		t.Fatalf("SendTextMulti() error = %v, want max recipients error", err)
	}
	if fake.sendCalls != 0 {
		t.Fatalf("over-limit SendTextMulti reached adapter %d times", fake.sendCalls)
	}
}

func TestSendTextMultiRejectsGroupRecipientWithoutSending(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	c := newStartedConnectedTestClient(t, events, fake)

	_, err := c.SendTextMulti(`["15550000001@s.whatsapp.net","120363000000000000@g.us"]`, "hello", "multi-group")
	if err == nil || !strings.Contains(err.Error(), "1:1 WhatsApp user JID") {
		t.Fatalf("SendTextMulti() error = %v, want group recipient rejection", err)
	}
	if fake.sendCalls != 0 {
		t.Fatalf("group recipient SendTextMulti reached adapter %d times", fake.sendCalls)
	}
}

func TestSendTextMultiDoesNotBackoffSecondRecipient(t *testing.T) {
	restore := overrideMultiSendTestTiming()
	defer restore()

	activeOperationMinInterval = time.Minute
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResultsByTarget = map[string]sendTextResult{
		"15550000001@s.whatsapp.net": {ServerMessageID: "server-1", RecipientJID: "15550000001@s.whatsapp.net"},
		"15550000002@s.whatsapp.net": {ServerMessageID: "server-2", RecipientJID: "15550000002@s.whatsapp.net"},
	}
	c := newStartedConnectedTestClient(t, events, fake)

	got, err := c.SendTextMulti(`["15550000001@s.whatsapp.net","15550000002@s.whatsapp.net"]`, "hello", "multi-backoff")
	if err != nil {
		t.Fatalf("SendTextMulti() error = %v", err)
	}
	if fake.sendCalls != 2 {
		t.Fatalf("send calls = %d, want 2", fake.sendCalls)
	}
	if strings.Contains(got, "operation_backoff") {
		t.Fatalf("second recipient hit operation backoff: %s", got)
	}
}

func TestSendTextMultiContinuesAfterSingleRecipientFailure(t *testing.T) {
	restore := overrideMultiSendTestTiming()
	defer restore()

	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResultsByTarget = map[string]sendTextResult{
		"15550000001@s.whatsapp.net": {ServerMessageID: "server-1", RecipientJID: "15550000001@s.whatsapp.net"},
		"15550000003@s.whatsapp.net": {ServerMessageID: "server-3", RecipientJID: "15550000003@s.whatsapp.net"},
	}
	fake.sendErrorsByTarget = map[string]error{
		"15550000002@s.whatsapp.net": errors.New("temporary recipient failure"),
	}
	c := newStartedConnectedTestClient(t, events, fake)

	got, err := c.SendTextMulti(`[
		"15550000001@s.whatsapp.net",
		"15550000002@s.whatsapp.net",
		"15550000003@s.whatsapp.net"
	]`, "hello", "multi-partial")
	if err != nil {
		t.Fatalf("SendTextMulti() error = %v", err)
	}

	var results []multiSendResult
	if err := json.Unmarshal([]byte(got), &results); err != nil {
		t.Fatalf("results JSON invalid: %v; %s", err, got)
	}
	if len(results) != 3 {
		t.Fatalf("results len = %d, want 3: %s", len(results), got)
	}
	if !results[0].OK || results[1].OK || !results[2].OK {
		t.Fatalf("unexpected per-recipient status: %#v", results)
	}
	if results[1].Error == "" {
		t.Fatalf("failed result missing error: %#v", results[1])
	}
}

func TestSendTextMultiResultIsRedacted(t *testing.T) {
	restore := overrideMultiSendTestTiming()
	defer restore()

	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResultsByTarget = map[string]sendTextResult{
		"15550000001@s.whatsapp.net": {ServerMessageID: "server-1", RecipientJID: "15550000001@s.whatsapp.net"},
	}
	fake.sendErrorsByTarget = map[string]error{
		"15550000002@s.whatsapp.net": errors.New("failed for 15550000002@s.whatsapp.net with secret body"),
	}
	c := newStartedConnectedTestClient(t, events, fake)

	got, err := c.SendTextMulti(`["15550000001@s.whatsapp.net","15550000002@s.whatsapp.net"]`, "secret body", "multi-redact")
	if err != nil {
		t.Fatalf("SendTextMulti() error = %v", err)
	}
	for _, forbidden := range []string{
		"15550000001",
		"15550000002",
		"15550000001@s.whatsapp.net",
		"15550000002@s.whatsapp.net",
		"secret body",
	} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("SendTextMulti result leaked %q: %s", forbidden, got)
		}
	}
}

func TestGetContactsReturnsJSON(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	got, err := c.GetContacts()
	if err != nil {
		t.Fatalf("GetContacts() error = %v", err)
	}
	if !strings.Contains(got, `"jid":"15551234567@s.whatsapp.net"`) {
		t.Fatalf("GetContacts() missing contact JID: %s", got)
	}
}

func TestGetGroupsReturnsJSON(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()

	c := newStartedConnectedTestClient(t, events, fake)

	got, err := c.GetGroups()
	if err != nil {
		t.Fatalf("GetGroups() error = %v", err)
	}
	if !strings.Contains(got, `"jid":"120363000000000000@g.us"`) {
		t.Fatalf("GetGroups() missing group JID: %s", got)
	}
	if !strings.Contains(got, `"participant_count":3`) {
		t.Fatalf("GetGroups() missing participant count: %s", got)
	}
}

func TestFreshLinkedDeviceCooldownBlocksContactsAndSend(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, false, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)
	if err := c.markFreshLinkedDevice(time.Now()); err != nil {
		t.Fatalf("markFreshLinkedDevice() error = %v", err)
	}

	if _, err := c.GetContacts(); err == nil || !strings.Contains(err.Error(), "fresh linked device cooldown") {
		t.Fatalf("GetContacts() error = %v, want fresh link cooldown", err)
	}
	if err := c.SendText("15551234567@s.whatsapp.net", "hello", "client-cooldown"); err == nil {
		t.Fatal("SendText() error = nil, want fresh link cooldown")
	}
	if fake.sendCalls != 0 {
		t.Fatalf("fresh cooldown send reached adapter %d times", fake.sendCalls)
	}
	got := events.waitFor(t, EventMessageFailed)
	if !strings.Contains(got.payload, `"error_code":"fresh_link_cooldown"`) {
		t.Fatalf("message_failed missing fresh_link_cooldown: %s", got.payload)
	}
}

func TestRiskStopBlocksConnectAndActiveOperations(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendErr = errors.New("server returned error 463")

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	if err := c.SendText("15551234567@s.whatsapp.net", "hello", "client-risk"); err == nil {
		t.Fatal("SendText() error = nil, want server error")
	}
	events.waitFor(t, EventMessageFailed)
	got := events.waitFor(t, EventRiskStopped)
	if !strings.Contains(got.payload, `"where":"sendText"`) {
		t.Fatalf("risk_stopped payload missing where=sendText: %s", got.payload)
	}
	if !fake.disconnected {
		t.Fatal("risk stop did not disconnect adapter")
	}
	if err := c.Connect(); err == nil || !strings.Contains(err.Error(), "risk stop active") {
		t.Fatalf("Connect() error = %v, want risk stop", err)
	}
}

func TestActiveOperationBackoffBlocksRapidCalls(t *testing.T) {
	oldInterval := activeOperationMinInterval
	activeOperationMinInterval = time.Minute
	defer func() { activeOperationMinInterval = oldInterval }()

	events := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)

	if _, err := c.GetContacts(); err != nil {
		t.Fatalf("GetContacts() error = %v", err)
	}
	if err := c.SendText("15551234567@s.whatsapp.net", "hello", "client-backoff"); err == nil {
		t.Fatal("SendText() error = nil, want operation backoff")
	}
	got := events.waitFor(t, EventMessageFailed)
	if !strings.Contains(got.payload, `"error_code":"operation_backoff"`) {
		t.Fatalf("message_failed missing operation_backoff: %s", got.payload)
	}
	if fake.sendCalls != 0 {
		t.Fatalf("operation backoff send reached adapter %d times", fake.sendCalls)
	}
}

func TestIncomingOneToOneTextEmitsPayloadButTraceIsRedacted(t *testing.T) {
	eventsRec := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(eventsRec, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fake.handler(&waevents.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("15551234567", types.DefaultUserServer),
				Sender: types.NewJID("15551234567", types.DefaultUserServer),
			},
			ID:        "incoming-1",
			Timestamp: time.Now(),
		},
		Message: &waE2E.Message{Conversation: proto.String("visible only in realtime callback")},
	})

	got := eventsRec.waitFor(t, EventMessageReceived)
	if !strings.Contains(got.payload, "visible only in realtime callback") {
		t.Fatalf("message_received callback payload missing text: %s", got.payload)
	}

	tracePath := filepath.Join(t.TempDir(), "trace.json")
	if err := c.ExportTrace(tracePath); err != nil {
		t.Fatalf("ExportTrace() error = %v", err)
	}
	traceBytes, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	trace := string(traceBytes)
	if strings.Contains(trace, "visible only in realtime callback") || strings.Contains(trace, "15551234567") {
		t.Fatalf("trace leaked received message details: %s", trace)
	}
}

func TestIncomingLIDOneToOneTextEmitsPayload(t *testing.T) {
	eventsRec := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(eventsRec, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fake.handler(&waevents.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("241820349030637", types.HiddenUserServer),
				Sender: types.NewJID("241820349030637", types.HiddenUserServer),
			},
			ID:        "incoming-lid-1",
			Timestamp: time.Now(),
		},
		Message: &waE2E.Message{Conversation: proto.String("hello from lid")},
	})

	got := eventsRec.waitFor(t, EventMessageReceived)
	if !strings.Contains(got.payload, `"from_jid":"241820349030637@lid"`) {
		t.Fatalf("message_received callback payload missing LID sender: %s", got.payload)
	}
}

func TestReceiptKeepsEntryUntilReadAck(t *testing.T) {
	eventsRec := newEventRecorder()
	c, err := NewClient(eventsRec, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.setState(StateConnected)
	c.mu.Lock()
	c.sentAt["server-1"] = time.Now().Add(-time.Second)
	c.mu.Unlock()

	c.handleReceipt(&waevents.Receipt{
		MessageIDs: []types.MessageID{types.MessageID("server-1")},
		Type:       waevents.ReceiptTypeDelivered,
	})

	delivered := eventsRec.waitFor(t, EventMessageAck)
	if !strings.Contains(delivered.payload, `"ack_level":1`) {
		t.Fatalf("delivered ack payload missing ack_level=1: %s", delivered.payload)
	}
	c.mu.Lock()
	_, ok := c.sentAt["server-1"]
	c.mu.Unlock()
	if !ok {
		t.Fatal("sentAt entry was deleted before read ack")
	}

	c.handleReceipt(&waevents.Receipt{
		MessageIDs: []types.MessageID{types.MessageID("server-1")},
		Type:       waevents.ReceiptTypeRead,
	})

	read := eventsRec.waitFor(t, EventMessageAck)
	if !strings.Contains(read.payload, `"ack_level":2`) {
		t.Fatalf("read ack payload missing ack_level=2: %s", read.payload)
	}
	c.mu.Lock()
	_, ok = c.sentAt["server-1"]
	c.mu.Unlock()
	if ok {
		t.Fatal("sentAt entry was not deleted after read ack")
	}
}

type recordedEvent struct {
	eventType string
	payload   string
}

type eventRecorder struct {
	mu     sync.Mutex
	events []recordedEvent
	ch     chan recordedEvent
}

func newEventRecorder() *eventRecorder {
	return &eventRecorder{ch: make(chan recordedEvent, 16)}
}

func newStartedConnectedTestClient(t *testing.T, events *eventRecorder, fake *fakeWAAdapter) *Client {
	t.Helper()
	c, err := NewClient(events, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, true, nil
	}
	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	c.setState(StateConnected)
	return c
}

func overrideMultiSendTestTiming() func() {
	oldInterval := activeOperationMinInterval
	oldDelay := multiSendDelay
	oldSleep := multiSendSleep
	activeOperationMinInterval = 0
	multiSendDelay = func() time.Duration { return 0 }
	multiSendSleep = func(time.Duration) {}
	return func() {
		activeOperationMinInterval = oldInterval
		multiSendDelay = oldDelay
		multiSendSleep = oldSleep
	}
}

func (r *eventRecorder) OnEvent(eventType string, payloadJSON string) {
	ev := recordedEvent{eventType: eventType, payload: payloadJSON}
	r.mu.Lock()
	r.events = append(r.events, ev)
	r.mu.Unlock()
	r.ch <- ev
}

func (r *eventRecorder) seen(eventType string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, ev := range r.events {
		if ev.eventType == eventType {
			return true
		}
	}
	return false
}

func (r *eventRecorder) types() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.events))
	for _, ev := range r.events {
		out = append(out, ev.eventType)
	}
	return out
}

func (r *eventRecorder) waitFor(t *testing.T, eventType string) recordedEvent {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev := <-r.ch:
			if ev.eventType == eventType {
				return ev
			}
		case <-deadline:
			t.Fatalf("timed out waiting for %s; events=%v", eventType, r.types())
		}
	}
}

type fakeWAAdapter struct {
	calls               []string
	qr                  chan qrItem
	handler             func(any)
	panicOnConnect      bool
	sendCalls           int
	sendTargets         []string
	sendResult          sendTextResult
	sendErr             error
	sendResultsByTarget map[string]sendTextResult
	sendErrorsByTarget  map[string]error
	waitForSendContext  bool
	reconnectCalls      int
	disconnected        bool
	closed              bool
}

func newFakeWAAdapter() *fakeWAAdapter {
	return &fakeWAAdapter{qr: make(chan qrItem, 4)}
}

func (f *fakeWAAdapter) AddEventHandler(handler func(any)) uint32 {
	f.calls = append(f.calls, "add_event_handler")
	f.handler = handler
	return 1
}

func (f *fakeWAAdapter) GetQRChannel(context.Context) (<-chan qrItem, error) {
	f.calls = append(f.calls, "get_qr_channel")
	return f.qr, nil
}

func (f *fakeWAAdapter) ConnectContext(context.Context) error {
	f.calls = append(f.calls, "connect")
	if f.panicOnConnect {
		panic("adapter connect exploded")
	}
	return nil
}

func (f *fakeWAAdapter) ReconnectAfterLogin(context.Context) error {
	f.calls = append(f.calls, "reconnect_after_login")
	f.reconnectCalls++
	return nil
}

func (f *fakeWAAdapter) Disconnect() { f.disconnected = true }

func (f *fakeWAAdapter) Close() error {
	f.closed = true
	return nil
}

func (f *fakeWAAdapter) UserIDString() string { return "" }

func (f *fakeWAAdapter) SendText(ctx context.Context, phone string, text string, clientMsgId string) (sendTextResult, error) {
	f.sendCalls++
	f.sendTargets = append(f.sendTargets, phone)
	if f.waitForSendContext {
		<-ctx.Done()
		return f.sendResult, ctx.Err()
	}
	if err, ok := f.sendErrorsByTarget[phone]; ok {
		return f.sendResultsByTarget[phone], err
	}
	if result, ok := f.sendResultsByTarget[phone]; ok {
		return result, nil
	}
	return f.sendResult, f.sendErr
}

func (f *fakeWAAdapter) GetContacts(context.Context) ([]contactInfo, error) {
	return []contactInfo{{JID: "15551234567@s.whatsapp.net", Name: "Test Contact"}}, nil
}

func (f *fakeWAAdapter) GetGroups(context.Context) ([]groupInfo, error) {
	return []groupInfo{{JID: "120363000000000000@g.us", Name: "Test Group", ParticipantCount: 3}}, nil
}

func (f *fakeWAAdapter) ResolveJID(context.Context, string) (string, error) {
	return "15551234567@s.whatsapp.net", nil
}
