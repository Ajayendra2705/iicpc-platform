package client

import "context"

// OrderClient is the common interface for all transport protocols (REST, WS, FIX).
type OrderClient interface {
	// PlaceOrder sends a new order and returns the assigned ID, RTT in ns, and any error.
	PlaceOrder(ctx context.Context, side string, price float64, qty int) (id string, latNs int64, err error)
	// CancelOrder cancels an existing order by ID.
	CancelOrder(ctx context.Context, id string) (latNs int64, err error)
	// Close releases any underlying connection resources.
	Close() error
}
