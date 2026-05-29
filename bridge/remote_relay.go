package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	EventRemoteRelayStarted      = "remote_relay_started"
	EventRemoteRelayConnected    = "remote_relay_connected"
	EventRemoteRelayDisconnected = "remote_relay_disconnected"
	EventRemoteRelayStopped      = "remote_relay_stopped"
	EventRemoteRelayError        = "remote_relay_error"

	remoteRelayMinTokenLength = 32
	remoteRelayWriteTimeout   = 15 * time.Second
	remoteRelayMinBackoff     = time.Second
	remoteRelayMaxBackoff     = time.Minute
)

type remoteCommandFrame struct {
	Type        string   `json:"type"`
	RequestID   string   `json:"request_id"`
	ToJids      []string `json:"to_jids,omitempty"`
	Text        string   `json:"text,omitempty"`
	ClientMsgID string   `json:"client_msg_id,omitempty"`
}

type remoteResponseFrame struct {
	Type       string `json:"type"`
	RequestID  string `json:"request_id"`
	OK         bool   `json:"ok"`
	ResultJSON string `json:"result_json,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
}

type remoteRelayClient struct {
	client *Client
	wsURL  string
	token  string

	mu      sync.Mutex
	writeMu sync.Mutex
	cancel  context.CancelFunc
	conn    *websocket.Conn
	running bool
}

func (c *Client) StartRemoteRelay(wsURL string, token string) (err error) {
	defer c.recoverAsError("StartRemoteRelay", &err)

	wsURL = strings.TrimSpace(wsURL)
	token = strings.TrimSpace(token)
	if err := validateRemoteRelayConfig(wsURL, token); err != nil {
		return err
	}

	relay := &remoteRelayClient{client: c, wsURL: wsURL, token: token}
	c.mu.Lock()
	previous := c.remoteRelay
	c.remoteRelay = relay
	c.mu.Unlock()
	if previous != nil {
		previous.stop()
	}

	relay.start()
	c.emit(EventRemoteRelayStarted, map[string]any{
		"url_host": remoteRelayHost(wsURL),
	}, map[string]any{
		"url_host": remoteRelayHost(wsURL),
	})
	return nil
}

func (c *Client) StopRemoteRelay() (err error) {
	defer c.recoverAsError("StopRemoteRelay", &err)

	c.mu.Lock()
	relay := c.remoteRelay
	c.remoteRelay = nil
	c.mu.Unlock()
	if relay != nil {
		relay.stop()
		c.emit(EventRemoteRelayStopped, map[string]any{}, map[string]any{})
	}
	return nil
}

func (c *Client) RemoteRelayStatus() (statusJSON string, err error) {
	defer c.recoverAsError("RemoteRelayStatus", &err)

	c.mu.Lock()
	relay := c.remoteRelay
	c.mu.Unlock()
	status := map[string]any{"enabled": relay != nil, "connected": false}
	if relay != nil {
		status["url_host"] = remoteRelayHost(relay.wsURL)
		status["connected"] = relay.connected()
	}
	b, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) handleRemoteCommand(frame remoteCommandFrame) (response remoteResponseFrame) {
	response = remoteResponseFrame{Type: "response", RequestID: frame.RequestID}
	defer func() {
		if r := recover(); r != nil {
			c.emit(EventRemoteRelayError, map[string]any{
				"where": "handleRemoteCommand",
			}, map[string]any{
				"where": "handleRemoteCommand",
			})
			response.OK = false
			response.ResultJSON = ""
			response.ErrorCode = "phone_error"
		}
	}()

	switch frame.Type {
	case "contacts":
		result, err := c.GetContacts()
		if err != nil {
			response.ErrorCode = remoteRelayErrorCode(err)
			return response
		}
		response.OK = true
		response.ResultJSON = result
		return response
	case "send":
		toJidsJSON, err := json.Marshal(frame.ToJids)
		if err != nil {
			response.ErrorCode = "invalid_request"
			return response
		}
		result, err := c.SendTextMulti(string(toJidsJSON), frame.Text, frame.ClientMsgID)
		if err != nil {
			response.ErrorCode = remoteRelayErrorCode(err)
			return response
		}
		response.OK = true
		response.ResultJSON = result
		return response
	default:
		response.ErrorCode = "invalid_request"
		return response
	}
}

func (r *remoteRelayClient) start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel = cancel
	r.running = true
	r.mu.Unlock()
	go r.run(ctx)
}

func (r *remoteRelayClient) stop() {
	r.mu.Lock()
	cancel := r.cancel
	conn := r.conn
	r.cancel = nil
	r.conn = nil
	r.running = false
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if conn != nil {
		_ = conn.Close(websocket.StatusNormalClosure, "stopped")
	}
}

func (r *remoteRelayClient) connected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.conn != nil
}

func (r *remoteRelayClient) run(ctx context.Context) {
	backoff := remoteRelayMinBackoff
	for {
		if ctx.Err() != nil {
			return
		}
		err := r.connectOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		r.setConn(nil)
		r.client.emit(EventRemoteRelayDisconnected, map[string]any{
			"url_host": remoteRelayHost(r.wsURL),
		}, map[string]any{
			"url_host": remoteRelayHost(r.wsURL),
		})
		if err != nil {
			r.client.emit(EventRemoteRelayError, map[string]any{
				"where": "remoteRelay",
				"code":  remoteRelayErrorCode(err),
			}, map[string]any{
				"where": "remoteRelay",
				"code":  remoteRelayErrorCode(err),
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > remoteRelayMaxBackoff {
			backoff = remoteRelayMaxBackoff
		}
	}
}

func (r *remoteRelayClient) connectOnce(ctx context.Context) error {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+r.token)
	conn, _, err := websocket.Dial(ctx, r.wsURL, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		return err
	}
	r.setConn(conn)
	r.client.emit(EventRemoteRelayConnected, map[string]any{
		"url_host": remoteRelayHost(r.wsURL),
	}, map[string]any{
		"url_host": remoteRelayHost(r.wsURL),
	})
	defer conn.Close(websocket.StatusNormalClosure, "reconnect")

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		go r.handleFrame(data)
	}
}

func (r *remoteRelayClient) setConn(conn *websocket.Conn) {
	r.mu.Lock()
	r.conn = conn
	r.mu.Unlock()
}

func (r *remoteRelayClient) handleFrame(data []byte) {
	defer func() {
		if recover() != nil {
			r.client.emit(EventRemoteRelayError, map[string]any{
				"where": "remoteRelayFrame",
			}, map[string]any{
				"where": "remoteRelayFrame",
			})
		}
	}()

	var frame remoteCommandFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		r.writeResponse(remoteResponseFrame{Type: "response", ErrorCode: "invalid_request"})
		return
	}
	response := r.client.handleRemoteCommand(frame)
	r.writeResponse(response)
}

func (r *remoteRelayClient) writeResponse(response remoteResponseFrame) {
	payload, err := json.Marshal(response)
	if err != nil {
		return
	}
	r.mu.Lock()
	conn := r.conn
	r.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteRelayWriteTimeout)
	defer cancel()
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	_ = conn.Write(ctx, websocket.MessageText, payload)
}

func validateRemoteRelayConfig(wsURL string, token string) error {
	if len(token) < remoteRelayMinTokenLength {
		return fmt.Errorf("remote relay token must be at least %d characters", remoteRelayMinTokenLength)
	}
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return fmt.Errorf("parse remote relay URL: %w", err)
	}
	if parsed.Scheme != "wss" {
		return errors.New("remote relay URL must use wss")
	}
	if parsed.Host == "" || parsed.Path != "/ws" {
		return errors.New("remote relay URL must point to /ws")
	}
	return nil
}

func remoteRelayHost(wsURL string) string {
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func remoteRelayErrorCode(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "exceeds max"):
		return "too_many_recipients"
	case strings.Contains(message, "recipient must be"):
		return "invalid_recipient"
	case strings.Contains(message, "risk"):
		return "risk_stopped"
	case strings.Contains(message, "cooldown"):
		return "cooldown"
	case strings.Contains(message, "send limit"):
		return "send_limit_exceeded"
	case strings.Contains(message, "not connected") || strings.Contains(message, "want \"connected\""):
		return "not_connected"
	default:
		return "phone_error"
	}
}
