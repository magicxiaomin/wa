package bridge

import (
	"encoding/json"
	"os"
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
