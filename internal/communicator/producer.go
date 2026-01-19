package communicator

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bilal/switchify-agent/internal/config"
	"github.com/rs/zerolog/log"
	"github.com/segmentio/kafka-go"
)

// KafkaProducer is responsible ONLY for Kafka interactions
type KafkaProducer struct {
	metricsWriter *kafka.Writer
	logsWriter    *kafka.Writer
}

// NewKafkaProducer initializes Kafka writers
func NewKafkaProducer(cfg *config.Config) (*KafkaProducer, error) {
	if len(cfg.Kafka.Brokers) == 0 {
		return nil, errors.New("kafka brokers not configured")
	}

	metricsWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      cfg.Kafka.Brokers,
		Topic:        "switchify.metrics",
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: int(kafka.RequireOne),
		Async:        true,
		BatchTimeout: 500 * time.Millisecond,
	})

	logsWriter := kafka.NewWriter(kafka.WriterConfig{
		Brokers:      cfg.Kafka.Brokers,
		Topic:        "switchify.logs",
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: int(kafka.RequireOne),
		Async:        true,
		BatchTimeout: 500 * time.Millisecond,
	})

	log.Info().Msg("kafka producer initialized")

	return &KafkaProducer{
		metricsWriter: metricsWriter,
		logsWriter:    logsWriter,
	}, nil
}

// PublishMetric publishes a metrics payload to Kafka
func (p *KafkaProducer) PublishMetric(ctx context.Context, m MetricsPayload) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return p.metricsWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(m.CorrelationID),
		Value: data,
	})
}

// PublishLog publishes a log payload to Kafka
func (p *KafkaProducer) PublishLog(ctx context.Context, l LogPayload) error {
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}

	return p.logsWriter.WriteMessages(ctx, kafka.Message{
		Key:   []byte(l.CorrelationID),
		Value: data,
	})
}

// Close shuts down Kafka writers gracefully
func (p *KafkaProducer) Close() error {
	log.Info().Msg("closing kafka producer")

	if err := p.metricsWriter.Close(); err != nil {
		return err
	}
	if err := p.logsWriter.Close(); err != nil {
		return err
	}
	return nil
}
