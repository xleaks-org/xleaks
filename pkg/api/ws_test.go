package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWSHubRejectsConnectionsOverLimit(t *testing.T) {
	oldMaxClients := wsMaxClients
	wsMaxClients = 1
	defer func() {
		wsMaxClients = oldMaxClients
	}()

	hub := NewWSHub()
	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("first websocket dial error = %v, status = %d", err, resp.StatusCode)
		}
		t.Fatalf("first websocket dial error = %v", err)
	}
	defer conn.Close()

	waitForWSClientCount(t, hub, 1)

	secondConn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		secondConn.Close()
		t.Fatal("expected second websocket dial to be rejected")
	}
	if resp == nil {
		t.Fatal("expected HTTP response for rejected websocket dial")
	}
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("rejected websocket status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	waitForWSClientCount(t, hub, 0)
}

func TestWSHubClosesOversizedClientMessage(t *testing.T) {
	oldReadLimit := wsClientReadLimit
	wsClientReadLimit = 32
	defer func() {
		wsClientReadLimit = oldReadLimit
	}()

	hub := NewWSHub()
	server := httptest.NewServer(http.HandlerFunc(hub.HandleWebSocket))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("websocket dial error = %v, status = %d", err, resp.StatusCode)
		}
		t.Fatalf("websocket dial error = %v", err)
	}
	defer conn.Close()

	waitForWSClientCount(t, hub, 1)

	payload := strings.Repeat("a", int(wsClientReadLimit)+1)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	waitForWSClientCount(t, hub, 0)
}

func waitForWSClientCount(t *testing.T, hub *WSHub, want int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := hub.ClientCount(); got == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("ClientCount() = %d, want %d", hub.ClientCount(), want)
}
