package realtime

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type SnapshotFunc func(userID int64) any

type client struct {
	userID int64
	conn   *websocket.Conn
	mu     sync.Mutex
}

type Hub struct {
	mu      sync.Mutex
	clients map[string]map[*client]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[string]map[*client]struct{})}
}

func (h *Hub) Serve(spaceID string, userID int64, snapshot SnapshotFunc, originPatterns []string, w http.ResponseWriter, r *http.Request) error {
	var options *websocket.AcceptOptions
	if len(originPatterns) > 0 {
		options = &websocket.AcceptOptions{OriginPatterns: originPatterns}
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return err
	}
	item := &client{userID: userID, conn: conn}
	h.add(spaceID, item)
	defer func() {
		h.remove(spaceID, item)
		_ = conn.Close(websocket.StatusNormalClosure, "closed")
	}()
	if err := item.write(snapshot(userID)); err != nil {
		return nil
	}
	for {
		if _, _, err := conn.Read(r.Context()); err != nil {
			return nil
		}
	}
}

func (h *Hub) Broadcast(spaceID string, snapshot SnapshotFunc) {
	h.mu.Lock()
	items := make([]*client, 0, len(h.clients[spaceID]))
	for item := range h.clients[spaceID] {
		items = append(items, item)
	}
	h.mu.Unlock()
	for _, item := range items {
		if err := item.write(snapshot(item.userID)); err != nil {
			h.remove(spaceID, item)
			_ = item.conn.Close(websocket.StatusGoingAway, "write failed")
		}
	}
}

func (h *Hub) add(spaceID string, item *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.clients[spaceID] == nil {
		h.clients[spaceID] = make(map[*client]struct{})
	}
	h.clients[spaceID][item] = struct{}{}
}

func (h *Hub) remove(spaceID string, item *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients[spaceID], item)
	if len(h.clients[spaceID]) == 0 {
		delete(h.clients, spaceID)
	}
}

func (c *client) write(value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return wsjsonWrite(ctx, c.conn, value)
}

func wsjsonWrite(ctx context.Context, conn *websocket.Conn, value any) error {
	return conn.Write(ctx, websocket.MessageText, mustJSON(value))
}
