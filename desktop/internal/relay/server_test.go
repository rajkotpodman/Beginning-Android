package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRelayServerWebSocketExchange(t *testing.T) {
	srv := NewServer(0, "TEST-PC", "amber-breeze-cedar-drift-echo-frost", "")
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", srv.handleWebSocket)
	mux.HandleFunc("/health", srv.handleHealth)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// 1. Test /health
	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer res.Body.Close()
	var health map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&health); err != nil {
		t.Fatalf("Failed to decode health: %v", err)
	}
	if health["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %v", health["status"])
	}

	// 2. Test WebSocket connection & Room Broadcast
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws?room=testroom"

	c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Client 1 failed to connect: %v", err)
	}
	defer c1.Close()

	c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Client 2 failed to connect: %v", err)
	}
	defer c2.Close()

	testMsg := []byte(`{"type":"clipboard","payload":"hello world"}`)
	if err := c1.WriteMessage(websocket.TextMessage, testMsg); err != nil {
		t.Fatalf("Client 1 write failed: %v", err)
	}

	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := c2.ReadMessage()
	if err != nil {
		t.Fatalf("Client 2 read failed: %v", err)
	}

	if string(msg) != string(testMsg) {
		t.Errorf("Expected message %s, got %s", string(testMsg), string(msg))
	}
}
