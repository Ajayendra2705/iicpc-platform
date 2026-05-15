package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/client"
)

func TestPlaceOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/order" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "ord-42"})
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	id, latNs, err := c.PlaceOrder(context.Background(), "buy", 100.50, 5)
	if err != nil {
		t.Fatalf("PlaceOrder: %v", err)
	}
	if id != "ord-42" {
		t.Fatalf("id: got %q want %q", id, "ord-42")
	}
	if latNs <= 0 {
		t.Fatal("latNs must be positive")
	}
}

func TestPlaceOrderServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	_, _, err := c.PlaceOrder(context.Background(), "buy", 100, 1)
	if err == nil {
		t.Fatal("expected error on 500")
	}
}

func TestCancelOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	latNs, err := c.CancelOrder(context.Background(), "ord-1")
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if latNs <= 0 {
		t.Fatal("latNs must be positive")
	}
}

func TestCancelOrderAcceptsNoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	_, err := c.CancelOrder(context.Background(), "ord-2")
	if err != nil {
		t.Fatalf("CancelOrder 204: %v", err)
	}
}

func TestCancelOrderServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := client.New(client.Config{BaseURL: srv.URL})
	_, err := c.CancelOrder(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error on 404")
	}
}
