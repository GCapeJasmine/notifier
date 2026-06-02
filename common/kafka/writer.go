package kafka

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/common/utils"
)

const (
	DefaultWriteTimeoutSeconds = 10
	// DefaultBatchTimeoutMilliseconds must be greater than 0
	DefaultBatchTimeoutMilliseconds = 1
)

var (
	balancer *kafka.Hash
)

func init() {
	balancer = &kafka.Hash{}
}

type WriterConfig struct {
	Auth         *AuthConfig   `yaml:"auth" mapstructure:"auth"`
	GroupID      string        `yaml:"group_id" mapstructure:"group_id"`
	Topic        string        `yaml:"topic" mapstructure:"topic"`
	WriteTimeout time.Duration `yaml:"write_timeout" mapstructure:"write_timeout"`
	BatchTimeout time.Duration `yaml:"batch_timeout" mapstructure:"batch_timeout"`
	Dialer       *kafka.Dialer
}

//go:generate mockery --name Writer --output ./../mock --filename writer.go --with-expecter
type Writer interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	WriteMessage(ctx context.Context, msg kafka.Message) error
	Close() error
}

type writer struct {
	kafkaWriter *kafka.Writer
}

func NewWriter(config WriterConfig) Writer {
	dialer := NewDialer(config.Auth)
	return NewDefaultWriter(dialer, &config)
}

func NewDefaultWriter(dialer *kafka.Dialer, cfg *WriterConfig) Writer {
	writeTimeout := DefaultWriteTimeoutSeconds * time.Second
	if cfg.WriteTimeout > 0 {
		writeTimeout = time.Duration(cfg.WriteTimeout) * time.Second
	}

	batchTimeout := DefaultBatchTimeoutMilliseconds * time.Millisecond
	if cfg.BatchTimeout > 0 {
		batchTimeout = time.Duration(cfg.BatchTimeout) * time.Millisecond
	}

	kafkaWriter := &kafka.Writer{
		Addr:     kafka.TCP(utils.SplitAndTrim(cfg.Auth.Url)...),
		Topic:    cfg.Topic,
		Balancer: balancer,
		Logger: &kafkaLogger{
			Logger: log.Logger,
		},
		ErrorLogger: &kafkaErrLogger{
			Logger: log.Logger,
		},
		WriteTimeout: writeTimeout,
		BatchTimeout: batchTimeout,
		Transport: &kafka.Transport{
			SASL:        dialer.SASLMechanism,
			DialTimeout: dialer.Timeout,
		},
		AllowAutoTopicCreation: true,
	}

	return &writer{
		kafkaWriter: kafkaWriter,
	}
}

func (r *writer) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(f float64) {
		log.Logger.Debugf("write messages eslapsed %f", f)
		writeMessageLatency.Observe(f)
	}))
	defer timer.ObserveDuration()

	return r.kafkaWriter.WriteMessages(ctx, msgs...)
}

func (r *writer) WriteMessage(ctx context.Context, msg kafka.Message) error {
	return r.kafkaWriter.WriteMessages(ctx, msg)
}

func (r *writer) Close() error {
	return r.kafkaWriter.Close()
}
