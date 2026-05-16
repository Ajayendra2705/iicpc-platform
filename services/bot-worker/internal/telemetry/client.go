// Package telemetry streams per-order OrderEvents from the bot-worker to the
// telemetry-ingester. Emit is non-blocking: events are buffered and dropped
// when the buffer is full, so order placement is never slowed by telemetry.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// Client is the bot-side telemetry emitter.
type Client interface {
	Emit(e *telemetryv1.OrderEvent)
	Close() error
}

// Stub discards events; used when telemetry is disabled.
type Stub struct{}

func NewStub() Stub                       { return Stub{} }
func (Stub) Emit(*telemetryv1.OrderEvent) {}
func (Stub) Close() error                 { return nil }

// GRPCClient buffers events and streams them to the ingester via a long-lived
// client-streaming RPC. A single goroutine drains the buffer; on send errors
// the stream is re-opened with backoff.
type GRPCClient struct {
	conn    *grpc.ClientConn
	api     telemetryv1.TelemetryIngesterClient
	queue   chan *telemetryv1.OrderEvent
	done    chan struct{}
	closed  atomic.Bool
	emitted atomic.Uint64
	dropped atomic.Uint64
	log     *slog.Logger
	wg      sync.WaitGroup
}

// Dial connects to addr, starts the background sender, and returns a Client.
func Dial(ctx context.Context, addr string, bufferCap int, log *slog.Logger) (*GRPCClient, error) {
	if bufferCap <= 0 {
		bufferCap = 1024
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %s: %w", addr, err)
	}
	c := &GRPCClient{
		conn:  conn,
		api:   telemetryv1.NewTelemetryIngesterClient(conn),
		queue: make(chan *telemetryv1.OrderEvent, bufferCap),
		done:  make(chan struct{}),
		log:   log,
	}
	c.wg.Add(1)
	go c.run(ctx)
	return c, nil
}

// NewForTest builds a client from an existing connection. Used by bufconn
// tests; production code calls Dial.
func NewForTest(conn *grpc.ClientConn, bufferCap int, log *slog.Logger) *GRPCClient {
	if bufferCap <= 0 {
		bufferCap = 1024
	}
	c := &GRPCClient{
		conn:  conn,
		api:   telemetryv1.NewTelemetryIngesterClient(conn),
		queue: make(chan *telemetryv1.OrderEvent, bufferCap),
		done:  make(chan struct{}),
		log:   log,
	}
	c.wg.Add(1)
	go c.run(context.Background())
	return c
}

// Emit enqueues e. Non-blocking; drops on full or closed buffer.
func (c *GRPCClient) Emit(e *telemetryv1.OrderEvent) {
	if c.closed.Load() {
		return
	}
	select {
	case c.queue <- e:
		c.emitted.Add(1)
	default:
		c.dropped.Add(1)
	}
}

// Close stops the sender and tears down the connection.
func (c *GRPCClient) Close() error {
	if !c.closed.CompareAndSwap(false, true) {
		return nil
	}
	close(c.done)
	c.wg.Wait()
	return c.conn.Close()
}

// Stats returns lifetime emit/drop counts.
func (c *GRPCClient) Stats() (emitted, dropped uint64) {
	return c.emitted.Load(), c.dropped.Load()
}

func (c *GRPCClient) run(ctx context.Context) {
	defer c.wg.Done()

	var (
		stream  telemetryv1.TelemetryIngester_IngestStreamClient
		backoff = 100 * time.Millisecond
	)

	openStream := func() error {
		s, err := c.api.IngestStream(ctx)
		if err != nil {
			return err
		}
		stream = s
		backoff = 100 * time.Millisecond
		return nil
	}

	for {
		select {
		case <-c.done:
			if stream != nil {
				_, _ = stream.CloseAndRecv()
			}
			return
		case <-ctx.Done():
			if stream != nil {
				_, _ = stream.CloseAndRecv()
			}
			return
		case e := <-c.queue:
			if stream == nil {
				if err := openStream(); err != nil {
					c.log.Warn("telemetry stream open", "err", err)
					if !c.sleepBackoff(&backoff) {
						return
					}
					continue
				}
			}
			if err := stream.Send(e); err != nil {
				c.log.Warn("telemetry send", "err", err)
				_, _ = stream.CloseAndRecv()
				stream = nil
				if !c.sleepBackoff(&backoff) {
					return
				}
			}
		}
	}
}

// sleepBackoff sleeps then doubles the backoff (cap 5s). Returns false if
// the client is shutting down so the caller exits run.
func (c *GRPCClient) sleepBackoff(b *time.Duration) bool {
	select {
	case <-time.After(*b):
		*b *= 2
		if *b > 5*time.Second {
			*b = 5 * time.Second
		}
		return true
	case <-c.done:
		return false
	}
}

// compile-time interface check
var _ Client = (*GRPCClient)(nil)
var _ Client = Stub{}

// ErrClosed is returned by Emit after Close.
var ErrClosed = errors.New("telemetry client closed")
