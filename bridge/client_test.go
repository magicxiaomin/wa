package bridge

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStartAndConnectMountHandlersAndQRBeforeNetworkConnect(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()

	c, err := NewClient(events.callback, t.TempDir(), "wa-test-device")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	c.newWA = func(context.Context, string, string) (waAdapter, bool, error) {
		return fake, false, nil
	}

	if err := c.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !events.seen("bridge_started") {
		t.Fatalf("Start() did not emit bridge_started; events=%v", events.types())
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

	got := events.waitFor(t, "qr_generated")
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

	c, err := NewClient(events.callback, t.TempDir(), "wa-test-device")
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

func TestStartInitializesModerncSQLiteSessionStore(t *testing.T) {
	events := newEventRecorder()
	dataDir := t.TempDir()

	c, err := NewClient(events.callback, dataDir, "wa-test-device")
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

	c, err := NewClient(events.callback, t.TempDir(), "wa-test-device")
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

func TestSendTextForTestRejectsNonWhitelistedRecipient(t *testing.T) {
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	c, err := NewClient(events.callback, t.TempDir(), "wa-test-device")
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
	events := newEventRecorder()
	fake := newFakeWAAdapter()
	fake.sendResult = sendTextResult{ServerMessageID: "3EB0OK", RecipientJID: "15551234567@s.whatsapp.net"}

	c, err := NewClient(events.callback, t.TempDir(), "wa-test-device")
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

func (r *eventRecorder) callback(eventType string, payloadJSON string) {
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
	calls          []string
	qr             chan qrItem
	handler        func(any)
	panicOnConnect bool
	sendCalls      int
	sendResult     sendTextResult
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

func (f *fakeWAAdapter) Disconnect() {}

func (f *fakeWAAdapter) Close() error { return nil }

func (f *fakeWAAdapter) UserIDString() string { return "" }

func (f *fakeWAAdapter) SendText(context.Context, string, string, string) (sendTextResult, error) {
	f.sendCalls++
	return f.sendResult, nil
}
