package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportTraceIncludesRawResearchFields(t *testing.T) {
	var trace traceRecorder
	trace.add(EventQRGenerated, StateWaitingQR, map[string]any{
		"qr":     "2@secret-qr-payload",
		"qr_len": 23,
	})
	trace.add(EventMessageSendStart, StateConnected, map[string]any{
		"clientMsgId": "client-1",
		"text":        "secret message body",
		"text_len":    19,
		"to":          "15551234567",
		"to_suffix":   "...4693",
	})
	trace.add(EventError, StateConnected, map[string]any{
		"where":   "test",
		"message": "auth token key session credential 14155552671",
	})

	path := filepath.Join(t.TempDir(), "trace.json")
	if err := trace.export(path); err != nil {
		t.Fatalf("export() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	out := string(raw)
	for _, required := range []string{
		"2@secret-qr-payload",
		"secret message body",
		"15551234567",
		"14155552671",
		"auth token key session credential",
	} {
		if !strings.Contains(out, required) {
			t.Fatalf("exported trace missing %q: %s", required, out)
		}
	}
	if !strings.Contains(out, `"qr_len": 23`) {
		t.Fatalf("exported trace dropped safe qr_len: %s", out)
	}
	if !strings.Contains(out, `"to_suffix": "...4693"`) {
		t.Fatalf("exported trace dropped safe suffix: %s", out)
	}
}

func TestTraceRecorderCapsEvents(t *testing.T) {
	var trace traceRecorder
	for i := 0; i < maxTraceEvents+3; i++ {
		trace.add(EventContactsSynced, StateConnected, nil)
	}

	path := filepath.Join(t.TempDir(), "trace.json")
	if err := trace.export(path); err != nil {
		t.Fatalf("export() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var events []traceEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(events) != maxTraceEvents {
		t.Fatalf("exported events len = %d, want %d", len(events), maxTraceEvents)
	}
	if events[0].Seq != 4 {
		t.Fatalf("first seq = %d, want 4", events[0].Seq)
	}
	if events[len(events)-1].Seq != maxTraceEvents+3 {
		t.Fatalf("last seq = %d, want %d", events[len(events)-1].Seq, maxTraceEvents+3)
	}
}
