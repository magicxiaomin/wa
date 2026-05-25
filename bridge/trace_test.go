package bridge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExportTraceSanitizesSensitiveFields(t *testing.T) {
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
	for _, forbidden := range []string{
		"2@secret-qr-payload",
		"secret message body",
		"15551234567",
		"14155552671",
		"auth token key session credential",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("exported trace leaked %q: %s", forbidden, out)
		}
	}
	if !strings.Contains(out, `"qr_len": 23`) {
		t.Fatalf("exported trace dropped safe qr_len: %s", out)
	}
	if !strings.Contains(out, `"to_suffix": "...4693"`) {
		t.Fatalf("exported trace dropped safe suffix: %s", out)
	}
}
