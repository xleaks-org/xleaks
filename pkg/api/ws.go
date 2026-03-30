package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/xleaks-org/xleaks/pkg/api/middleware"
	"github.com/xleaks-org/xleaks/pkg/metrics"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		return middleware.OriginAllowed(r, origin)
	},
}

var (
	wsClientReadLimit = int64(4 << 10)
	wsMaxClients      = 256
)

const (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
)

// WSEvent represents a real-time event pushed to the frontend.
type WSEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WSHub manages all active WebSocket connections and broadcasts events.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
	pending int
}

type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*wsClient]bool),
	}
}

// HandleWebSocket upgrades an HTTP connection to WebSocket and registers the client.
func (hub *WSHub) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	if !hub.reserveSlot() {
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		hub.releasePendingSlot()
		slog.Error("websocket upgrade failed", "error", err)
		return
	}
	conn.SetReadLimit(wsClientReadLimit)
	if err := conn.SetReadDeadline(time.Now().Add(wsPongWait)); err != nil {
		hub.releasePendingSlot()
		conn.Close()
		return
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 256),
	}

	hub.mu.Lock()
	hub.pending--
	hub.clients[client] = true
	hub.mu.Unlock()

	metrics.IncrWS()

	go client.writePump()
	go client.readPump(hub)
}

// Broadcast sends an event to all connected WebSocket clients.
func (hub *WSHub) Broadcast(event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		slog.Error("failed to marshal websocket event", "error", err)
		return
	}

	var stale []*wsClient

	hub.mu.RLock()
	for client := range hub.clients {
		select {
		case client.send <- data:
		default:
			// Client's send buffer is full; mark for removal.
			stale = append(stale, client)
		}
	}
	hub.mu.RUnlock()

	if len(stale) > 0 {
		hub.mu.Lock()
		for _, client := range stale {
			// Guard against double-close: only act if the client is still registered.
			if hub.clients[client] {
				close(client.send)
				delete(hub.clients, client)
			}
		}
		hub.mu.Unlock()
	}
}

// Close gracefully shuts down all WebSocket connections by sending a close
// frame and releasing resources.
func (hub *WSHub) Close() {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for client := range hub.clients {
		client.conn.WriteMessage(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutting down"))
		client.conn.Close()
		close(client.send)
		delete(hub.clients, client)
	}
}

// ClientCount returns the number of connected WebSocket clients.
func (hub *WSHub) ClientCount() int {
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	return len(hub.clients)
}

func (hub *WSHub) reserveSlot() bool {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if len(hub.clients)+hub.pending >= wsMaxClients {
		return false
	}
	hub.pending++
	return true
}

func (hub *WSHub) releasePendingSlot() {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	if hub.pending > 0 {
		hub.pending--
	}
}

func (c *wsClient) writePump() {
	ticker := time.NewTicker(wsPingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
				return
			}
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait)); err != nil {
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (c *wsClient) readPump(hub *WSHub) {
	defer func() {
		hub.mu.Lock()
		delete(hub.clients, c)
		hub.mu.Unlock()
		c.conn.Close()
		metrics.DecrWS()
	}()

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		// We don't process incoming messages from clients in v1.0.
		// The WebSocket is primarily for server-to-client push.
	}
}
