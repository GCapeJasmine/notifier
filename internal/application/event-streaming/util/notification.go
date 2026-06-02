package util

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/gleo/subscribers/pkg/workflow"
)

const (
	DeleteOp       EventOp = "d"
	CreateOp       EventOp = "c"
	UpdateOp       EventOp = "u"
	AddToSegmentOp EventOp = "a"
)

type NotifyEvent struct {
	Key   NotifyEventKey
	Value NotifyEventValue
}

type NotifyEventValue struct {
	Payload NotifyEventPayload `json:"payload"`
}

type NotifyEventPayload struct {
	EventId    string          `json:"event_id"`
	Op         EventOp         `json:"op"`
	TenantId   string          `json:"tenant_id"`
	Subscriber json.RawMessage `json:"subscriber"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type NotifyEventKey struct {
	TenantId     string `json:"tenant_id,omitempty"`
	SubscriberId string `json:"subscriber_id"`
}

type Subscriber struct {
	SubscriberId string `json:"subscriber_id"`
}

type EventOp string

func (e EventOp) IsCrud() bool {
	return e == DeleteOp || e == CreateOp || e == UpdateOp || e == AddToSegmentOp
}

type NotifyEventParser struct {
	next workflow.Processor
}

func NewEventParser() *NotifyEventParser {
	return &NotifyEventParser{}
}

func (n *NotifyEventParser) SetNext(processor workflow.Processor) {
	n.next = processor
}

func (n *NotifyEventParser) Process(ctx context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	results := make([]workflow.WorkflowItem, 0)
	for _, item := range items {
		domain, err := n.parse(item)
		if err != nil {
			return nil, err
		}
		if domain != nil {
			results = append(results, workflow.WorkflowItem{Payload: *domain})
		}
	}
	return n.next.Process(ctx, results)
}

func (n *NotifyEventParser) parse(item workflow.WorkflowItem) (*NotifyEvent, error) {
	wrapped, ok := item.Payload.(workflow.KafkaMessage)
	if !ok {
		return nil, errors.New("item payload is not kafka message")
	}

	message := wrapped.Message
	if len(message.Value) == 0 {
		return nil, nil
	}

	var (
		key   NotifyEventKey
		value NotifyEventValue
	)
	if err := json.Unmarshal(message.Value, &value); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(message.Key, &key); err != nil {
		return nil, err
	}

	return &NotifyEvent{
		Key:   key,
		Value: value,
	}, nil
}
