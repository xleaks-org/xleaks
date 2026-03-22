package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Local-only API, CORS handled by middleware.
	},
}

// WSEvent represents a real-time event pushed to the frontend.
type WSEvent struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// WSHub manages all active WebSocket connections and broadcasts events.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]bool
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
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 256),
	}

	hub.mu.Lock()
	hub.clients[client] = true
	hub.mu.Unlock()

	go client.writePump()
	go client.readPump(hub)
}

// Broadcast sends an event to all connected WebSocket clients.
func (hub *WSHub) Broadcast(event WSEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		log.Printf("failed to marshal ws event: %v", err)
		return
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()

	for client := range hub.clients {
		select {
		case client.send <- data:
		default:
			// Client's send buffer is full, close it.
			close(client.send)
			delete(hub.clients, client)
		}
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

func (c *wsClient) writePump() {
	defer c.conn.Close()
	for message := range c.send {
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			return
		}
	}
}

func (c *wsClient) readPump(hub *WSHub) {
	defer func() {
		hub.mu.Lock()
		delete(hub.clients, c)
		hub.mu.Unlock()
		c.conn.Close()
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
