package telemetry_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/bot-worker/internal/telemetry"
)

// mockServer counts events received over IngestStream.
type mockServer struct {
	telemetryv1.UnimplementedTelemetryIngesterServer
	mu       sync.Mutex
	received []*telemetryv1.OrderEvent
	count    atomic.Int64
}

func (m *mockServer) IngestStream(stream telemetryv1.TelemetryIngester_IngestStreamServer) error {
	for {
		e, err := stream.Recv()
		if err != nil {
			return stream.SendAndClose(&telemetryv1.IngestAck{EventsReceived: m.count.Load()})
		}
		m.mu.Lock()
		m.received = append(m.received, e)
		m.mu.Unlock()
		m.count.Add(1)
	}
}

func (m *mockServer) snapshot() []*telemetryv1.OrderEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]*telemetryv1.OrderEvent, len(m.received))
	copy(cp, m.received)
	return cp
}

func setup(t *testing.T) (*telemetry.GRPCClient, *mockServer, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gs := grpc.NewServer()
	srv := &mockServer{}
	telemetryv1.RegisterTelemetryIngesterServer(gs, srv)
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	client := telemetry.NewForTest(conn, 64, log)

	return client, srv, func() {
		_ = client.Close()
		gs.Stop()
	}
}

func TestEmitDeliversAcrossStream(t *testing.T) {
	c, srv, cleanup := setup(t)
	defer cleanup()

	for i := range 10 {
		c.Emit(&telemetryv1.OrderEvent{
			OrderId:      "ord-" + string(rune('0'+i)),
			ContestantId: "alpha",
			LatencyNs:    int64(i + 1),
		})
	}

	// wait for stream to drain
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(srv.snapshot()) >= 10 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := srv.snapshot()
	if len(got) != 10 {
		t.Errorf("received: got %d want 10", len(got))
	}
}

func TestStubEmitsAreNoOps(t *testing.T) {
	s := telemetry.NewStub()
	for range 100 {
		s.Emit(&telemetryv1.OrderEvent{})
	}
	if err := s.Close(); err != nil {
		t.Errorf("stub Close: %v", err)
	}
}

func TestEmitAfterCloseDoesNotPanic(t *testing.T) {
	c, _, cleanup := setup(t)
	cleanup()
	c.Emit(&telemetryv1.OrderEvent{}) // must not panic
}

func TestCloseIdempotent(t *testing.T) {
	c, _, cleanup := setup(t)
	defer cleanup()
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close (2nd): %v", err)
	}
}
