package util

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kafka "github.com/segmentio/kafka-go"

	"github.com/gleo/subscribers/pkg/workflow"
)

// stubProcessor records what it receives and returns it unchanged.
type stubProcessor struct {
	received []workflow.WorkflowItem
	err      error
}

func (s *stubProcessor) SetNext(_ workflow.Processor) {}
func (s *stubProcessor) Process(_ context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	s.received = items
	return items, s.err
}

func makeKafkaMessage(key, value string) workflow.KafkaMessage {
	km := workflow.KafkaMessage{
		Topic: "subscribers.events",
		Key:   key,
		Value: json.RawMessage(value),
	}
	km.Message = kafka.Message{
		Key:   []byte(key),
		Value: []byte(value),
	}
	return km
}

func TestEventParser_ValidMessage_ProducesNotifyEvent(t *testing.T) {
	key := `{"tenant_id":"t1","subscriber_id":"s1"}`
	value := `{"payload":{"event_id":"e1","op":"c","tenant_id":"t1","subscriber":{"subscriber_id":"s1"},"occurred_at":"2024-01-01T00:00:00Z"}}`

	next := &stubProcessor{}
	parser := NewEventParser()
	parser.SetNext(next)

	km := makeKafkaMessage(key, value)
	items := workflow.NewWorkflowItems([]workflow.KafkaMessage{km})
	_, err := parser.Process(context.Background(), items)
	require.NoError(t, err)

	require.Len(t, next.received, 1)

	// Regression: payload must be NotifyEvent value type, not *NotifyEvent
	evt, ok := next.received[0].Payload.(NotifyEvent)
	require.True(t, ok, "payload should be NotifyEvent (value), not *NotifyEvent (pointer)")
	assert.Equal(t, "t1", evt.Key.TenantId)
	assert.Equal(t, "s1", evt.Key.SubscriberId)
	assert.Equal(t, CreateOp, evt.Value.Payload.Op)
}

func TestEventParser_EmptyValue_SkipsItem(t *testing.T) {
	next := &stubProcessor{}
	parser := NewEventParser()
	parser.SetNext(next)

	km := workflow.KafkaMessage{}
	km.Message = kafka.Message{Key: []byte("k"), Value: []byte("")}
	items := workflow.NewWorkflowItems([]workflow.KafkaMessage{km})

	_, err := parser.Process(context.Background(), items)
	require.NoError(t, err)
	assert.Empty(t, next.received)
}

func TestEventParser_InvalidValueJSON_ReturnsError(t *testing.T) {
	next := &stubProcessor{}
	parser := NewEventParser()
	parser.SetNext(next)

	km := makeKafkaMessage(`{"tenant_id":"t1"}`, `not-valid-json`)
	items := workflow.NewWorkflowItems([]workflow.KafkaMessage{km})

	_, err := parser.Process(context.Background(), items)
	assert.Error(t, err)
}

func TestEventParser_EmptyItems_ReturnsNil(t *testing.T) {
	parser := NewEventParser()
	result, err := parser.Process(context.Background(), nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestEventOp_IsCrud(t *testing.T) {
	assert.True(t, CreateOp.IsCrud())
	assert.True(t, UpdateOp.IsCrud())
	assert.True(t, DeleteOp.IsCrud())
	assert.True(t, AddToSegmentOp.IsCrud())
	assert.False(t, EventOp("x").IsCrud())
}

func TestNotifyEventPayload_Fields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	p := NotifyEventPayload{
		EventId:    "e1",
		Op:         UpdateOp,
		TenantId:   "t1",
		Subscriber: json.RawMessage(`{"subscriber_id":"s1"}`),
		OccurredAt: now,
	}
	b, err := json.Marshal(p)
	require.NoError(t, err)

	var p2 NotifyEventPayload
	require.NoError(t, json.Unmarshal(b, &p2))
	assert.Equal(t, p.EventId, p2.EventId)
	assert.Equal(t, p.Op, p2.Op)
	assert.Equal(t, p.TenantId, p2.TenantId)
}
