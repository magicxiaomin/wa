package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/magicxiaomin/wa/bridge"
)

type eventCallbackFunc func(eventType string, payloadJSON string)

func (f eventCallbackFunc) OnEvent(eventType string, payloadJSON string) {
	f(eventType, payloadJSON)
}

func main() {
	dataDir := flag.String("data-dir", "./wa-session", "local directory for whatsmeow session store")
	deviceName := flag.String("device-name", "wa-desktop-poc", "device name shown in WhatsApp linked devices")
	sendTo := flag.String("send-to", "", "optional whitelisted recipient phone in full country-code format, without +")
	text := flag.String("text", "", "optional one-off text message to send after connected")
	clientMsgID := flag.String("client-msg-id", "", "optional caller-generated id for correlating send events")
	flag.Parse()

	events := make(chan string, 16)
	client, err := bridge.NewClient(eventCallbackFunc(func(eventType string, payloadJSON string) {
		fmt.Printf("%s %s\n", eventType, payloadJSON)
		select {
		case events <- eventType:
		default:
		}
		if eventType == "qr_generated" {
			var payload struct {
				QR string `json:"qr"`
			}
			if json.Unmarshal([]byte(payloadJSON), &payload) == nil && payload.QR != "" {
				if err := renderTerminalQR(os.Stdout, payload.QR); err != nil {
					fmt.Fprintf(os.Stderr, "render QR: %v\n", err)
				}
			}
		}
	}), *dataDir, *deviceName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "new client: %v\n", err)
		os.Exit(1)
	}

	if err := client.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := client.ExportTrace("trace.json"); err != nil {
			fmt.Fprintf(os.Stderr, "export trace: %v\n", err)
		}
		_ = client.Stop()
	}()

	if err := client.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}

	if *sendTo != "" || *text != "" {
		if *sendTo == "" || *text == "" {
			fmt.Fprintln(os.Stderr, "-send-to and -text must be provided together")
			os.Exit(1)
		}
		id := *clientMsgID
		if id == "" {
			id = fmt.Sprintf("cli-%d", time.Now().UnixNano())
		}
		if err := waitForConnected(events, 90*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "wait connected: %v\n", err)
			os.Exit(1)
		}
		if err := client.SendTextForTest(*sendTo, *text, id); err != nil {
			fmt.Fprintf(os.Stderr, "send text: %v\n", err)
			os.Exit(1)
		}
		if err := waitForSendResult(events, 90*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "wait send result: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Println("Waiting for QR scan / connected state. Press Ctrl+C to stop.")
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
}

func waitForConnected(events <-chan string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case eventType := <-events:
			if eventType == "connected" || eventType == "session_restored" {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timed out waiting for connected")
		}
	}
}

func waitForSendResult(events <-chan string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case eventType := <-events:
			switch eventType {
			case "message_sent":
				return nil
			case "message_failed":
				return fmt.Errorf("message_failed")
			}
		case <-timer.C:
			return fmt.Errorf("timed out waiting for message_sent")
		}
	}
}
