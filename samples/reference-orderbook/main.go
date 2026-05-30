package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"reference-orderbook/engine"
)

var (
	ob      = engine.New()
	orderID atomic.Uint64
)

type placeReq struct {
	ID    string  `json:"id"`
	Side  string  `json:"side"`
	Kind  string  `json:"kind"` // "limit" (default) or "market"
	Price float64 `json:"price"`
	Qty   int64   `json:"qty"`
}

func main() {
	port := os.Getenv("RUNTIME_PORT")
	if port == "" {
		port = "9100"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("POST /order", handlePlace)
	mux.HandleFunc("DELETE /order/{id}", handleCancel)
	mux.HandleFunc("GET /orderbook", handleSnapshot)

	srv := &http.Server{Addr: net.JoinHostPort("", port), Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		log.Printf("listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutCtx)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func handlePlace(w http.ResponseWriter, r *http.Request) {
	var req placeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var side engine.Side
	switch req.Side {
	case "buy":
		side = engine.Buy
	case "sell":
		side = engine.Sell
	default:
		http.Error(w, "side must be buy or sell", http.StatusBadRequest)
		return
	}
	id := req.ID
	if id == "" {
		id = fmt.Sprintf("ord-%d", orderID.Add(1))
	}
	kind := req.Kind
	if kind == "" {
		kind = "limit"
	}
	o := &engine.Order{ID: id, Side: side, Price: req.Price, Qty: req.Qty, At: time.Now()}
	var fills []engine.Fill
	var err error
	switch kind {
	case "limit":
		fills, err = ob.Place(o)
	case "market":
		fills, err = ob.PlaceMarket(o)
	default:
		http.Error(w, "kind must be limit or market", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "fills": fills})
}

func handleCancel(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := ob.Cancel(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	snap := ob.Snapshot()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}
