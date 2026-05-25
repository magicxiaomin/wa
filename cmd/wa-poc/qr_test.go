package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderTerminalQRIncludesVisibleQRAndPayloadFallback(t *testing.T) {
	var out bytes.Buffer

	if err := renderTerminalQR(&out, "2@test-qr-code"); err != nil {
		t.Fatalf("renderTerminalQR() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Scan this QR with WhatsApp Linked Devices") {
		t.Fatalf("render output missing scan instruction: %q", got)
	}
	if !strings.Contains(got, "2@test-qr-code") {
		t.Fatalf("render output missing payload fallback: %q", got)
	}
	if !strings.Contains(got, "\u2588") {
		t.Fatalf("render output does not look like a terminal QR: %q", got)
	}
}
