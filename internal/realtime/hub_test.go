package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestHubAllowsConfiguredOriginAndBroadcasts(t *testing.T) {
	hub := NewHub()
	var state atomic.Int64
	state.Store(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = hub.Serve("table", 7, func(int64) any {
			return map[string]int64{"state": state.Load()}
		}, []string{"https://poker.example.com"}, w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{"Origin": []string{"https://poker.example.com"}}
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	readState := func() int64 {
		_, data, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var message map[string]int64
		if err := json.Unmarshal(data, &message); err != nil {
			t.Fatal(err)
		}
		return message["state"]
	}
	if got := readState(); got != 1 {
		t.Fatalf("initial state = %d, want 1", got)
	}
	state.Store(2)
	hub.Broadcast("table", func(int64) any { return map[string]int64{"state": state.Load()} })
	if got := readState(); got != 2 {
		t.Fatalf("broadcast state = %d, want 2", got)
	}
}

func TestHubRejectsUnconfiguredOrigin(t *testing.T) {
	hub := NewHub()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = hub.Serve("table", 7, func(int64) any { return nil }, nil, w, r)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := http.Header{"Origin": []string{"https://unexpected.example.com"}}
	conn, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), &websocket.DialOptions{HTTPHeader: header})
	if conn != nil {
		conn.CloseNow()
	}
	if err == nil {
		t.Fatal("unexpected origin should be rejected")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected response: %#v", response)
	}
}
