package kafka

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/segmentio/kafka-go"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/common/utils"
)

const (
	DefaultMinBytes                = 1
	DefaultMaxBytes                = 10e6
	DefaultReadTimeoutMilliSeconds = 10
)

type Reader struct {
	*kafka.Reader
}

type ReaderConfig struct {
	Auth        *AuthConfig `yaml:"auth" mapstructure:"auth"`
	ReadTimeout uint        `yaml:"read_timeout" mapstructure:"read_timeout"`
	GroupID     string      `yaml:"group_id" mapstructure:"group_id"`
	Topic       string      `yaml:"topic" mapstructure:"topic"`
	GroupTopics []string    `yaml:"group_topics" mapstructure:"group_topics"`
	StartOffset int64       `yaml:"start_offset" mapstructure:"start_offset"`

	Dialer *kafka.Dialer
}

func NewReader(cfg ReaderConfig) *Reader {
	// check empty config, if no config, return a nil reader
	if cfg.Auth == nil {
		return nil
	}
	dialer := NewDialer(cfg.Auth)

	return NewDefaultReader(dialer, &cfg)
}

func NewDefaultReader(dialer *kafka.Dialer, cfg *ReaderConfig) *Reader {
	readerCfg := kafka.ReaderConfig{
		Brokers:  utils.SplitAndTrim(cfg.Auth.Url),
		Topic:    cfg.Topic,
		MinBytes: DefaultMinBytes,
		MaxBytes: DefaultMaxBytes,
		Logger: &kafkaLogger{
			Logger: log.Logger,
		},
		ErrorLogger: &kafkaErrLogger{
			Logger: log.Logger,
		},
		Dialer:           dialer,
		ReadBatchTimeout: DefaultReadTimeoutMilliSeconds * time.Millisecond,
		QueueCapacity:    1000,
	}

	if cfg.ReadTimeout > 0 {
		readerCfg.ReadBatchTimeout = time.Duration(cfg.ReadTimeout) * time.Millisecond
	}

	if cfg.GroupID != "" {
		readerCfg.GroupID = cfg.GroupID
	}

	if len(cfg.GroupTopics) > 0 {
		readerCfg.GroupTopics = cfg.GroupTopics
	}

	if cfg.StartOffset != 0 {
		readerCfg.StartOffset = cfg.StartOffset
	}

	reader := kafka.NewReader(readerCfg)

	return &Reader{
		reader,
	}
}

func (r *Reader) ReadLatestOffset(ctx context.Context) (int64, error) {
	config := r.Config()
	dialer := config.Dialer

	var (
		conn *kafka.Conn
		err  error
	)

	for _, broker := range config.Brokers {
		conn, err = dialer.DialLeader(ctx, "tcp", broker, config.Topic, 0)
	}

	if err != nil {
		return 0, err
	}
	defer conn.Close()

	lastOffset, err := conn.ReadLastOffset()
	if err != nil {
		return 0, err
	}

	// Latest = Last - 1
	return lastOffset - 1, nil
}

func (r *Reader) FetchLatestMessage(ctx context.Context) (*kafka.Message, error) {
	offset, err := r.ReadLatestOffset(ctx)
	if err != nil {
		return nil, err
	}

	// When offset equals -1 mean the current topic is empty
	if offset == -1 {
		return nil, nil
	}

	if err := r.SetOffset(offset); err != nil {
		return nil, err
	}

	lastMessage, err := r.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}

	return &lastMessage, nil
}

func (r *Reader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(f float64) {
		fetchMessageLatency.Observe(f)
	}))
	defer timer.ObserveDuration()

	message, err := r.Reader.FetchMessage(ctx)
	if err == nil {
		log.Logger.Debugf("fetched kafka partition %d, offset: %d, key: %s", message.Partition, message.Offset, strings.ToLower(string(message.Key[:])))
		fetchMessageLag.Observe(time.Since(message.Time).Seconds())
	}
	return message, err
}

func (r *Reader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	if len(msgs) == 0 {
		return nil
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(f float64) {
		commitMessageLatency.Observe(f)
	}))
	defer timer.ObserveDuration()
	err := r.Reader.CommitMessages(ctx, msgs...)
	if err == nil {
		now := time.Now()
		for _, msg := range msgs {
			commitMessageLag.Observe(now.Sub(msg.Time).Seconds())
		}
	}
	return err
}

func (r *Reader) FetchBatch(ctx context.Context, timeout time.Duration, maxSize int) ([]kafka.Message, error) {
	return pollKafka[kafka.Message](ctx, timeout, maxSize, r.FetchMessage)
}

func pollKafka[T any](ctx context.Context, timeout time.Duration, maxSize int,
	pollFunc func(context.Context) (T, error)) ([]T, error) {
	// Create child context with timeout
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if maxSize <= 0 {
		maxSize = 1
	}

	messages := make([]T, 0)
	for {
		select {
		case <-cctx.Done():
			return messages, nil
		default:
			message, err := pollFunc(cctx)
			switch err {
			case io.EOF:
				log.Logger.Infof("KafkaReader closed")
				return messages, nil
			case context.DeadlineExceeded:
				return messages, nil
			case nil:
				messages = append(messages, message)
				if len(messages) == maxSize {
					return messages, nil
				}
			default:
				return messages, err
			}
		}
	}
}
