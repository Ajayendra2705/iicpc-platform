package producer

import (
	"context"
	"fmt"

	"github.com/segmentio/kafka-go"
	"google.golang.org/protobuf/proto"

	telemetryv1 "github.com/Ajayendra2705/iicpc-platform/proto/gen/go/telemetry/v1"
)

// KafkaConfig configures the Redpanda/Kafka producer.
type KafkaConfig struct {
	Brokers []string
	Topic   string
}

// Kafka publishes protobuf-encoded OrderEvents to a Redpanda/Kafka topic,
// keyed by ContestantID for partition affinity.
type Kafka struct {
	w *kafka.Writer
}

func NewKafka(cfg KafkaConfig) *Kafka {
	return &Kafka{
		w: &kafka.Writer{
			Addr:                   kafka.TCP(cfg.Brokers...),
			Topic:                  cfg.Topic,
			Balancer:               &kafka.Hash{},
			BatchSize:              256,
			RequiredAcks:           kafka.RequireOne,
			AllowAutoTopicCreation: true,
		},
	}
}

func (p *Kafka) Publish(ctx context.Context, batch []*telemetryv1.OrderEvent) error {
	msgs := make([]kafka.Message, 0, len(batch))
	for _, e := range batch {
		data, err := proto.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal event %s: %w", e.GetOrderId(), err)
		}
		msgs = append(msgs, kafka.Message{
			Key:   []byte(e.GetContestantId()),
			Value: data,
		})
	}
	return p.w.WriteMessages(ctx, msgs...)
}

func (p *Kafka) Close() error { return p.w.Close() }

var _ Producer = (*Kafka)(nil)
