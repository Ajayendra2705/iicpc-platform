package main

import (
	"context"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"reference-orderbook/engine"
)

var ob = engine.New()

type placeReq struct {
	ID    string  `json:"id"`
	Side  string  `json:"side"`
	Price float64 `json:"price"`
	Qty   int64   `json:"qty"`
}

func main() {
	port := os.Getenv("RUNTIME_PORT")
	if port == "" {
		port = "8080"
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
	srv.Shutdown(shutCtx)
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
	o := &engine.Order{ID: req.ID, Side: side, Price: req.Price, Qty: req.Qty, At: time.Now()}
	fills, err := ob.Place(o)
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"fills": fills})
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
	json.NewEncoder(w).Encode(snap)
}
