package client

import "context"

// PlaceResult is the outcome of placing an order, returned by every transport.
// Filled is the contestant-reported fill quantity for this order; FillKnown is
// true only when the transport actually parsed authoritative fill data from the
// response (REST always; WS when the response carries a "filled" field; FIX
// does not parse ExecutionReport CumQty yet, so it reports FillKnown=false).
// The validator only scores fill-accuracy for events whose fill is authoritative.
type PlaceResult struct {
	ID        string
	Filled    int64
	FillKnown bool
	LatencyNs int64
}

// OrderClient is the common interface for all transport protocols (REST, WS, FIX).
type OrderClient interface {
	// PlaceOrder sends a limit order and returns the placement outcome and any error.
	PlaceOrder(ctx context.Context, side string, price float64, qty int) (PlaceResult, error)
	// PlaceMarketOrder sends a market order (no price, IOC) and returns the
	// placement outcome and any error.
	PlaceMarketOrder(ctx context.Context, side string, qty int) (PlaceResult, error)
	// CancelOrder cancels an existing order by ID.
	CancelOrder(ctx context.Context, id string) (latNs int64, err error)
	// Close releases any underlying connection resources.
	Close() error
}
