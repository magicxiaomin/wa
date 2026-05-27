package bridge

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type traceEvent struct {
	Timestamp string `json:"ts"`
	Seq       uint64 `json:"seq"`
	Event     string `json:"event"`
	State     string `json:"state"`
	Data      any    `json:"data"`
}

type traceRecorder struct {
	mu     sync.Mutex
	seq    uint64
	events []traceEvent
	next   int
}

const maxTraceEvents = 5000

func (t *traceRecorder) add(eventType string, state string, data any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.seq++
	if data == nil {
		data = map[string]any{}
	}
	event := traceEvent{
		Timestamp: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Seq:       t.seq,
		Event:     eventType,
		State:     state,
		Data:      data,
	}
	if len(t.events) < maxTraceEvents {
		t.events = append(t.events, event)
		return
	}
	t.events[t.next] = event
	t.next = (t.next + 1) % maxTraceEvents
}

func (t *traceRecorder) export(path string) error {
	t.mu.Lock()
	events := t.snapshotLocked()
	t.mu.Unlock()

	for i, event := range events {
		events[i] = sanitizeTraceEvent(event)
	}

	b, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func (t *traceRecorder) snapshotLocked() []traceEvent {
	events := make([]traceEvent, 0, len(t.events))
	if len(t.events) < maxTraceEvents || t.next == 0 {
		return append(events, t.events...)
	}
	events = append(events, t.events[t.next:]...)
	events = append(events, t.events[:t.next]...)
	return events
}

func sanitizeTraceEvent(event traceEvent) traceEvent {
	event.Data = sanitizeTraceData(event.Event, event.Data)
	return event
}

func sanitizeTraceData(eventType string, data any) any {
	in, ok := data.(map[string]any)
	if !ok {
		return map[string]any{}
	}

	out := make(map[string]any)
	allow := func(key string) {
		if value, exists := in[key]; exists {
			out[key] = sanitizeTraceValue(value)
		}
	}

	switch eventType {
	case EventQRGenerated:
		allow("qr_len")
	case EventConnected, EventPaired, EventSessionRestored:
		allow("jid_suffix")
	case EventMessageSendStart:
		allow("clientMsgId")
		allow("to_suffix")
		allow("text_len")
	case EventMessageSent:
		allow("clientMsgId")
		allow("server_msg_id")
		allow("latency_ms")
		allow("recipient_suffix")
		allow("recipient_server")
		allow("used_lid")
	case EventMessageAck:
		allow("server_msg_id")
		allow("ack_level")
		allow("latency_ms")
	case EventMessageFailed:
		allow("clientMsgId")
		allow("error_code")
		allow("error")
		allow("recipient_suffix")
		allow("recipient_server")
		allow("used_lid")
	case EventMessageReceived:
		allow("from_suffix")
		allow("text_len")
		allow("server_msg_id")
		allow("ts")
	case EventContactsSynced:
		return map[string]any{}
	case EventManualReconnect:
		return map[string]any{}
	case EventRiskStopped:
		allow("where")
		allow("reason")
		allow("retry_after_seconds")
	case EventDisconnected:
		allow("reason")
		allow("will_reconnect")
	case EventError:
		allow("where")
		allow("message")
	default:
		return map[string]any{}
	}
	return out
}

func sanitizeTraceValue(value any) any {
	switch v := value.(type) {
	case string:
		return sanitizeTraceString(v)
	default:
		return v
	}
}

var fullPhonePattern = regexp.MustCompile(`\b\d{8,15}\b`)

func sanitizeTraceString(value string) string {
	lower := strings.ToLower(value)
	for _, marker := range []string{"auth", "credential", "session", "token", "secret", " key"} {
		if strings.Contains(lower, marker) {
			return "[redacted]"
		}
	}
	return fullPhonePattern.ReplaceAllStringFunc(value, func(match string) string {
		if len(match) <= 4 {
			return "..." + match
		}
		return "..." + match[len(match)-4:]
	})
}
