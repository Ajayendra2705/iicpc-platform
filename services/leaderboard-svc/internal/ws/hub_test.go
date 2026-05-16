package ws_test

import (
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/leaderboard-svc/internal/ws"
)

func TestRegisterAndCount(t *testing.T) {
	h := ws.NewHub()
	if h.Count() != 0 {
		t.Errorf("initial Count: got %d want 0", h.Count())
	}
	c1 := h.Register()
	c2 := h.Register()
	if h.Count() != 2 {
		t.Errorf("Count after 2 registers: got %d want 2", h.Count())
	}
	h.Unregister(c1)
	h.Unregister(c2)
	if h.Count() != 0 {
		t.Errorf("Count after unregisters: got %d want 0", h.Count())
	}
}

func TestBroadcastDeliversToAll(t *testing.T) {
	h := ws.NewHub()
	c1 := h.Register()
	c2 := h.Register()

	h.Broadcast([]byte("hi"))

	for _, c := range []*ws.Client{c1, c2} {
		select {
		case msg := <-c.Send():
			if string(msg) != "hi" {
				t.Errorf("got %q want hi", string(msg))
			}
		default:
			t.Fatal("client did not receive broadcast")
		}
	}
}

func TestBroadcastDropsForSlowClient(t *testing.T) {
	h := ws.NewHub()
	c := h.Register()
	// fill the client's send buffer (default 16)
	for range 16 {
		h.Broadcast([]byte("x"))
	}
	// this 17th broadcast should drop silently for c
	h.Broadcast([]byte("dropped"))

	// drain the channel
	count := 0
	for {
		select {
		case <-c.Send():
			count++
		default:
			if count != 16 {
				t.Errorf("buffered: got %d want 16 (slow client should drop excess)", count)
			}
			return
		}
	}
}

func TestUnregisterClosesChannel(t *testing.T) {
	h := ws.NewHub()
	c := h.Register()
	h.Unregister(c)
	if _, ok := <-c.Send(); ok {
		t.Error("send channel should be closed after Unregister")
	}
}

func TestUnregisterIdempotent(t *testing.T) {
	h := ws.NewHub()
	c := h.Register()
	h.Unregister(c)
	// must not panic on double close
	h.Unregister(c)
}
