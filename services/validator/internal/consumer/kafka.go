package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

type KafkaConfig struct {
	Brokers []string
	Topic   string
	GroupID string
}

type Kafka struct {
	r   *kafka.Reader
	log *slog.Logger
}

func NewKafka(cfg KafkaConfig, log *slog.Logger) *Kafka {
	if cfg.GroupID == "" {
		cfg.GroupID = "validator"
	}
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.Brokers,
		Topic:   cfg.Topic,
		GroupID: cfg.GroupID,
	})
	return &Kafka{r: r, log: log}
}

func (c *Kafka) Consume(ctx context.Context, handler func(*telemetryv1.OrderEvent)) error {
	for {
		m, err := c.r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return fmt.Errorf("fetch: %w", err)
		}
		var e telemetryv1.OrderEvent
		if err := proto.Unmarshal(m.Value, &e); err != nil {
			c.log.Warn("decode event", "err", err, "offset", m.Offset)
			continue
		}
		handler(&e)
		if err := c.r.CommitMessages(ctx, m); err != nil {
			c.log.Warn("commit offset", "err", err, "offset", m.Offset)
		}
	}
}

func (c *Kafka) Close() error { return c.r.Close() }

var _ Consumer = (*Kafka)(nil)
