package server_test

import (
	"context"
	"io"
	"log/slog"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/buffer"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/server"
)

func setup(t *testing.T, bufCap int) (telemetryv1.TelemetryIngesterClient, *buffer.Buffer, func()) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	buf := buffer.New(bufCap)
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	gs := grpc.NewServer()
	telemetryv1.RegisterTelemetryIngesterServer(gs, server.New(buf, log))
	go func() { _ = gs.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough://bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cli := telemetryv1.NewTelemetryIngesterClient(conn)
	return cli, buf, func() {
		_ = conn.Close()
		gs.Stop()
		buf.Close()
	}
}

func TestIngestStreamHappy(t *testing.T) {
	cli, buf, cleanup := setup(t, 100)
	defer cleanup()

	stream, err := cli.IngestStream(context.Background())
	if err != nil {
		t.Fatalf("IngestStream: %v", err)
	}
	const N = 50
	for i := range N {
		if err := stream.Send(&telemetryv1.OrderEvent{OrderId: string(rune('a' + i%26)), ContestantId: "c1"}); err != nil {
			t.Fatalf("Send[%d]: %v", i, err)
		}
	}
	ack, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	if ack.GetEventsReceived() != N {
		t.Errorf("ack: got %d want %d", ack.GetEventsReceived(), N)
	}
	if s := buf.Stats(); s.Pushed != N {
		t.Errorf("buffer pushed: got %d want %d", s.Pushed, N)
	}
}

func TestIngestStreamDropsWhenBufferFull(t *testing.T) {
	cli, buf, cleanup := setup(t, 5) // tiny capacity
	defer cleanup()

	stream, err := cli.IngestStream(context.Background())
	if err != nil {
		t.Fatalf("IngestStream: %v", err)
	}
	const N = 50
	for i := range N {
		_ = stream.Send(&telemetryv1.OrderEvent{OrderId: string(rune('a' + i%26))})
	}
	ack, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv: %v", err)
	}
	// receive count reflects only accepted (pushed) events; drops are not counted
	if ack.GetEventsReceived() > 5 {
		t.Errorf("ack: got %d (over capacity)", ack.GetEventsReceived())
	}
	s := buf.Stats()
	if s.Dropped == 0 {
		t.Error("expected drops with tiny buffer, got 0")
	}
}

func TestQueryMetricsStub(t *testing.T) {
	cli, _, cleanup := setup(t, 100)
	defer cleanup()

	resp, err := cli.QueryMetrics(context.Background(), &telemetryv1.QueryMetricsRequest{ContestantId: "c-42"})
	if err != nil {
		t.Fatalf("QueryMetrics: %v", err)
	}
	if resp.GetContestantId() != "c-42" {
		t.Errorf("contestant_id echoed: got %q", resp.GetContestantId())
	}
}
