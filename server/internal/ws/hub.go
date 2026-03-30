package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"remoteaccess/server/internal/auth"
	"remoteaccess/server/internal/devices"
	"remoteaccess/server/internal/sessions"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 10 * 1024 * 1024
)

type Client struct {
	conn     *websocket.Conn
	hub      *Hub
	userID   string
	deviceID string
	send     chan []byte
}

type Hub struct {
	mu              sync.RWMutex
	clients         map[string]*Client
	tokens          *auth.TokenManager
	devicesService  *devices.Service
	sessionsService *sessions.Service
	allowedOrigins  map[string]struct{}
	allowAllOrigins bool
	upgrader        websocket.Upgrader
}

func NewHub(tokens *auth.TokenManager, devicesService *devices.Service, sessionsService *sessions.Service, allowedOrigins []string) *Hub {
	hub := &Hub{
		clients:         make(map[string]*Client),
		tokens:          tokens,
		devicesService:  devicesService,
		sessionsService: sessionsService,
		allowedOrigins:  make(map[string]struct{}, len(allowedOrigins)),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			Subprotocols:    []string{"ra.v1"},
			CheckOrigin:     nil,
		},
	}
	for _, origin := range allowedOrigins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			hub.allowAllOrigins = true
		}
		hub.allowedOrigins[trimmed] = struct{}{}
	}
	hub.upgrader.CheckOrigin = hub.checkOrigin
	return hub
}

func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	deviceID := strings.TrimSpace(r.URL.Query().Get("device_id"))
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	deviceKey := strings.TrimSpace(r.URL.Query().Get("device_key"))
	if token == "" || deviceKey == "" || deviceID == "" {
		subToken, subDeviceKey, subDeviceID := parseAuthFromSubprotocols(websocket.Subprotocols(r))
		if token == "" {
			token = subToken
		}
		if deviceKey == "" {
			deviceKey = subDeviceKey
		}
		if deviceID == "" {
			deviceID = subDeviceID
		}
	}

	if token == "" || deviceID == "" || deviceKey == "" {
		log.Printf(
			"ws rejected missing auth fields: device_id=%t token=%t device_key=%t origin=%q",
			deviceID != "",
			token != "",
			deviceKey != "",
			r.Header.Get("Origin"),
		)
		http.Error(w, "token, device_id and device_key are required", http.StatusBadRequest)
		return
	}

	claims, err := h.tokens.Parse(token)
	if err != nil || claims.Type != "access" {
		log.Printf("ws rejected invalid token for device %s: %v", deviceID, err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	_, err = h.devicesService.Authenticate(r.Context(), claims.UserID, deviceID, deviceKey)
	if err != nil {
		log.Printf("ws rejected device auth for user=%s device=%s: %v", claims.UserID, deviceID, err)
		http.Error(w, "device not found", http.StatusForbidden)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade error: %v", err)
		return
	}

	_ = h.devicesService.SetStatus(context.Background(), deviceID, "online")

	client := &Client{
		conn:     conn,
		hub:      h,
		userID:   claims.UserID,
		deviceID: deviceID,
		send:     make(chan []byte, 64),
	}

	h.register(client)
	go client.writePump()
	go client.readPump()

	welcome := OutgoingMessage{Type: "ws_ready", Payload: map[string]any{"device_id": deviceID}}
	bytes, _ := json.Marshal(welcome)
	client.send <- bytes
}

func (h *Hub) checkOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Desktop native clients can omit Origin.
		return true
	}
	if h.allowAllOrigins {
		return true
	}
	if _, ok := h.allowedOrigins[origin]; ok {
		return true
	}
	if isTauriOrigin(origin) {
		return true
	}
	log.Printf("ws rejected origin %q host=%q", origin, r.Host)
	return false
}

func (h *Hub) register(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if existing, ok := h.clients[client.deviceID]; ok {
		_ = existing.conn.Close()
	}
	h.clients[client.deviceID] = client
}

func (h *Hub) unregister(client *Client) {
	h.mu.Lock()
	removed := false
	if current, ok := h.clients[client.deviceID]; ok && current == client {
		delete(h.clients, client.deviceID)
		removed = true
	}
	h.mu.Unlock()

	close(client.send)
	_ = client.conn.Close()
	if removed {
		_ = h.devicesService.SetStatus(context.Background(), client.deviceID, "offline")
	}
}

func (h *Hub) NotifyDevice(deviceID string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	h.mu.RLock()
	client, ok := h.clients[deviceID]
	h.mu.RUnlock()
	if !ok {
		return nil
	}

	select {
	case client.send <- data:
	default:
		log.Printf("ws send channel full for device %s", deviceID)
	}
	return nil
}

func (c *Client) readPump() {
	defer c.hub.unregister(c)

	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("ws read error for device %s: %v", c.deviceID, err)
			}
			return
		}
		c.handleMessage(message)
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		_ = c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *Client) handleMessage(raw []byte) {
	var msg IncomingMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		c.sendError("invalid_json", "invalid websocket payload")
		return
	}

	switch msg.Type {
	case "ping":
		response, _ := json.Marshal(OutgoingMessage{Type: "pong"})
		c.send <- response
	case "heartbeat":
		_ = c.hub.devicesService.SetStatus(context.Background(), c.deviceID, "online")
	case "session_signal":
		c.handleSessionSignal(msg)
	default:
		c.sendError("unknown_type", "unknown message type")
	}
}

func (c *Client) handleSessionSignal(msg IncomingMessage) {
	session, peerDeviceID, err := c.hub.sessionsService.ValidateSessionParticipant(
		context.Background(),
		msg.SessionID,
		msg.SessionToken,
		c.deviceID,
	)
	if err != nil {
		c.sendError("session_invalid", err.Error())
		return
	}

	out := OutgoingMessage{
		Type:       "session_signal",
		FromDevice: c.deviceID,
		SessionID:  session.ID,
		Kind:       msg.Kind,
		Payload:    map[string]any{},
	}
	if len(msg.Payload) > 0 {
		var payload any
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			out.Payload = payload
		}
	}

	_ = c.hub.NotifyDevice(peerDeviceID, out)
}

func (c *Client) sendError(code, message string) {
	payload, _ := json.Marshal(OutgoingMessage{
		Type: "error",
		Payload: map[string]any{
			"code":    code,
			"message": message,
		},
	})
	select {
	case c.send <- payload:
	default:
	}
}

func parseAuthFromSubprotocols(values []string) (token string, deviceKey string, deviceID string) {
	for _, value := range values {
		switch {
		case strings.HasPrefix(value, "access."):
			token = strings.TrimPrefix(value, "access.")
		case strings.HasPrefix(value, "dkey."):
			deviceKey = strings.TrimPrefix(value, "dkey.")
		case strings.HasPrefix(value, "did."):
			deviceID = strings.TrimPrefix(value, "did.")
		}
	}
	return token, deviceKey, deviceID
}

func isTauriOrigin(origin string) bool {
	if strings.HasPrefix(origin, "tauri://") {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "tauri.localhost" || strings.HasSuffix(host, ".tauri.localhost")
}
