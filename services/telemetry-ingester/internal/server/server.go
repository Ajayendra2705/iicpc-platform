// Package server implements the TelemetryIngester gRPC service.
package server

import (
	"context"
	"errors"
	"io"
	"log/slog"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
	"github.com/Ajayendra2705/iicpc-platform/services/telemetry-ingester/internal/buffer"
)

// Server implements telemetryv1.TelemetryIngesterServer.
type Server struct {
	telemetryv1.UnimplementedTelemetryIngesterServer
	buf *buffer.Buffer
	log *slog.Logger
}

func New(buf *buffer.Buffer, log *slog.Logger) *Server {
	return &Server{buf: buf, log: log}
}

// IngestStream reads OrderEvents from a client-streaming RPC and pushes each
// onto the buffer. Returns IngestAck with the number of events accepted.
func (s *Server) IngestStream(stream telemetryv1.TelemetryIngester_IngestStreamServer) error {
	var received int64
	for {
		e, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return stream.SendAndClose(&telemetryv1.IngestAck{EventsReceived: received})
		}
		if err != nil {
			s.log.Warn("ingest recv", "err", err, "received", received)
			return err
		}
		if s.buf.Push(e) {
			received++
		}
	}
}

// QueryMetrics is stubbed in D15; aggregator service (D16) will populate this
// by consuming the published Kafka topic and computing rolling percentiles.
func (s *Server) QueryMetrics(_ context.Context, req *telemetryv1.QueryMetricsRequest) (*telemetryv1.QueryMetricsResponse, error) {
	return &telemetryv1.QueryMetricsResponse{ContestantId: req.GetContestantId()}, nil
}
