package main

import (
	"errors"
	"fmt"
	"io"

	qrcode "github.com/skip2/go-qrcode"
)

func renderTerminalQR(w io.Writer, payload string) error {
	if payload == "" {
		return errors.New("QR payload is empty")
	}

	code, err := qrcode.New(payload, qrcode.Medium)
	if err != nil {
		return err
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Scan this QR with WhatsApp Linked Devices:")
	for _, row := range code.Bitmap() {
		for _, dark := range row {
			if dark {
				fmt.Fprint(w, "██")
			} else {
				fmt.Fprint(w, "  ")
			}
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Payload fallback:")
	fmt.Fprintln(w, payload)
	fmt.Fprintln(w)
	return nil
}
