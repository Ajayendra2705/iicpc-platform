// Package ws is a small in-process fan-out hub for live leaderboard updates.
// Clients register with bounded send channels; Broadcast drops messages for
// clients that fall behind to keep the hub responsive.
package ws

import (
	"sync"
)

const defaultClientBuf = 16

// Client is a subscriber's outbound queue.
type Client struct {
	send chan []byte
}

// Send returns the read side of the client's queue; the HTTP handler reads
// from this and writes to the WS connection.
func (c *Client) Send() <-chan []byte { return c.send }

// Hub fans messages out to all registered clients.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]struct{}
}

func NewHub() *Hub { return &Hub{clients: make(map[*Client]struct{})} }

// Register creates a new client and adds it to the hub.
func (h *Hub) Register() *Client {
	c := &Client{send: make(chan []byte, defaultClientBuf)}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unregister removes the client and closes its send channel.
func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
	h.mu.Unlock()
}

// Broadcast pushes msg to every registered client; drops it for any client
// whose buffer is full (slow consumer protection).
func (h *Hub) Broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			// slow consumer; drop and move on
		}
	}
}

// Count returns the current number of registered clients.
func (h *Hub) Count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
