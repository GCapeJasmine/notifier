package workflow

import (
	"context"
	"encoding/json"
	"time"

	kafka2 "github.com/gleo/subscribers/common/kafka"
	"github.com/segmentio/kafka-go"
)

type EventSource[T any] interface {
	Fetch(ctx context.Context) ([]T, error)
	Close(ctx context.Context) error
	Commit(ctx context.Context, events []T) error
}

type KafkaMessage struct {
	Message       kafka.Message   `json:"-"`
	Topic         string          `json:"topic,omitempty"`
	Partition     int             `json:"partition,omitempty"`
	Offset        int64           `json:"offset,omitempty"`
	HighWaterMark int64           `json:"highWaterMark,omitempty"`
	Time          int64           `json:"time,omitempty"`
	Key           string          `json:"key,omitempty"`
	Value         json.RawMessage `json:"value,omitempty"`
}

func NewKafkaMessage(msg kafka.Message) KafkaMessage {
	return KafkaMessage{
		Message:       msg,
		Topic:         msg.Topic,
		Partition:     msg.Partition,
		Offset:        msg.Offset,
		HighWaterMark: msg.HighWaterMark,
		Time:          msg.Time.UnixNano(),
		Key:           string(msg.Key),
		Value:         msg.Value,
	}
}

func (k *KafkaMessage) UnmarshalJSON(bytes []byte) error {
	type km struct {
		Topic         string          `json:"topic,omitempty"`
		Partition     int             `json:"partition,omitempty"`
		Offset        int64           `json:"offset,omitempty"`
		HighWaterMark int64           `json:"highWaterMark,omitempty"`
		Time          int64           `json:"time,omitempty"`
		Key           string          `json:"key,omitempty"`
		Value         json.RawMessage `json:"value,omitempty"`
	}

	var kmObj km
	err := json.Unmarshal(bytes, &kmObj)
	if err != nil {
		return err
	}
	k.Message = kafka.Message{
		Topic:         kmObj.Topic,
		Partition:     kmObj.Partition,
		Offset:        kmObj.Offset,
		HighWaterMark: kmObj.HighWaterMark,
		Time:          time.Unix(0, kmObj.Time),
		Key:           []byte(kmObj.Key),
		Value:         kmObj.Value,
	}
	k.Topic = kmObj.Topic
	k.Partition = kmObj.Partition
	k.Offset = kmObj.Offset
	k.HighWaterMark = kmObj.HighWaterMark
	k.Time = kmObj.Time
	k.Key = kmObj.Key
	k.Value = kmObj.Value
	return nil
}

type SegmentIOSource struct {
	reader *kafka2.Reader
}

func NewSegmentIOSource(reader *kafka2.Reader) EventSource[KafkaMessage] {
	return &SegmentIOSource{reader: reader}
}

func (s *SegmentIOSource) Fetch(ctx context.Context) ([]KafkaMessage, error) {
	msg, err := s.reader.FetchMessage(ctx)
	if err != nil {
		return nil, err
	}
	return []KafkaMessage{NewKafkaMessage(msg)}, nil
}

func (s *SegmentIOSource) Close(ctx context.Context) error {
	return s.reader.Close()
}

func (s *SegmentIOSource) Commit(ctx context.Context, messages []KafkaMessage) error {
	if len(messages) == 0 {
		return nil
	}
	msg := make([]kafka.Message, len(messages))
	for i, _ := range messages {
		msg[i] = messages[i].Message
	}
	return s.reader.CommitMessages(ctx, msg...)
}

type SourceBatchReadConfig struct {
	Timeout   time.Duration `yaml:"timeout" mapstructure:"timeout" default:"500ms"`
	BatchSize int           `yaml:"batch_size" mapstructure:"batch_size" default:"100"`
}

type SegmentIOBatchSource struct {
	reader *kafka2.Reader
	config SourceBatchReadConfig
}

func NewSegmentIOBatchSource(reader *kafka2.Reader, config SourceBatchReadConfig) EventSource[KafkaMessage] {
	return &SegmentIOBatchSource{reader: reader, config: config}
}

func (s *SegmentIOBatchSource) Fetch(ctx context.Context) ([]KafkaMessage, error) {
	msgs, err := s.reader.FetchBatch(ctx, s.config.Timeout, s.config.BatchSize)
	if err != nil {
		return nil, err
	}
	if len(msgs) == 0 {
		return nil, nil
	}

	results := make([]KafkaMessage, len(msgs))
	for i, item := range msgs {
		results[i] = NewKafkaMessage(item)
	}
	return results, nil
}

func (s *SegmentIOBatchSource) Close(_ context.Context) error {
	return s.reader.Close()
}

func (s *SegmentIOBatchSource) Commit(ctx context.Context, messages []KafkaMessage) error {
	if len(messages) == 0 {
		return nil
	}
	msg := make([]kafka.Message, len(messages))
	for i, _ := range messages {
		msg[i] = messages[i].Message
	}
	return s.reader.CommitMessages(ctx, msg...)
}
