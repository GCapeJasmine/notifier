package dead_letter_replay

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	"github.com/gleo/subscribers/internal/application/event-streaming/util"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/pkg/workflow"
)

func makeDispatcher(suppliers map[string]WorkflowSupplier) *DeadLetterDispatcher {
	return NewDeadLetterDispatcher(&stubRepo{}, suppliers, config.EventStreamingConfig{})
}

// buildDeadLetterData produces the JSON that DeadLetterProcessor stores:
// []WorkflowItem{Payload: KafkaMessage} marshalled
func buildDeadLetterData(t *testing.T, km workflow.KafkaMessage) json.RawMessage {
	t.Helper()
	items := []workflow.WorkflowItem{{Payload: km}}
	b, err := json.Marshal(items)
	require.NoError(t, err)
	return b
}

// TestParseToDownStream_JsonTag validates the `json:"payload"` tag fix.
// Before the fix, Payload unmarshalled to zero-value KafkaMessage.
func TestParseToDownStream_JsonTag(t *testing.T) {
	km := workflow.KafkaMessage{
		Topic: "subscribers.events",
		Key:   `{"tenant_id":"t1","subscriber_id":"s1"}`,
		Value: json.RawMessage(`{"payload":{"op":"c","tenant_id":"t1"}}`),
	}
	letter := domain.DeadLetter{Data: buildDeadLetterData(t, km)}
	d := makeDispatcher(nil)

	items, err := d.parseToDownStream(letter)
	require.NoError(t, err)
	require.Len(t, items, 1)

	got, ok := items[0].Payload.(workflow.KafkaMessage)
	require.True(t, ok)
	assert.Equal(t, "subscribers.events", got.Topic)
	assert.Equal(t, km.Key, got.Key)
}

func TestParseToDownStream_InvalidJSON(t *testing.T) {
	d := makeDispatcher(nil)
	_, err := d.parseToDownStream(domain.DeadLetter{Data: json.RawMessage(`not-json`)})
	assert.Error(t, err)
}

func TestDispatcher_UnknownWorkflowName(t *testing.T) {
	d := makeDispatcher(map[string]WorkflowSupplier{})
	km := workflow.KafkaMessage{Topic: "t", Key: "k", Value: json.RawMessage(`{}`)}
	letter := domain.DeadLetter{
		ID:           1,
		WorkflowName: "unknown-workflow",
		Data:         buildDeadLetterData(t, km),
	}
	items := workflow.NewWorkflowItems([]domain.DeadLetter{letter})

	ctx := context.WithValue(context.Background(), workflow.WorkflowContextKey, &workflow.WorkflowContext{
		Id: "test", Name: "dead-letter-replay", Attr: map[string]any{},
	})
	_, err := d.Process(ctx, items)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown-workflow")
}

func TestDispatcher_RoutesToCorrectWorkflow(t *testing.T) {
	proc := &stubProcessor{}
	called := 0

	supplier := func(_ config.EventStreamingConfig) (*workflow.Workflow[workflow.KafkaMessage], error) {
		called++
		wf, err := workflow.NewWorkflowBuilder[workflow.KafkaMessage]().
			Source(nil).
			AddProcessor(proc).
			Name("Webhook Notifier").
			Build()
		// Build requires a source; inject directly via GetRoot workaround
		_ = wf
		_ = err
		return nil, errors.New("stub supplier: not needed") // supplier won't be called in this path
	}
	_ = supplier

	// Use getWorkflow directly with a cached root
	d := makeDispatcher(nil)
	d.roots["Webhook Notifier"] = proc

	km := workflow.KafkaMessage{Topic: "t", Key: "k", Value: json.RawMessage(`{"payload":{"op":"c","tenant_id":"t1","subscriber":{},"occurred_at":"2024-01-01T00:00:00Z","event_id":"e1"}}`)}
	letter := domain.DeadLetter{
		ID:           42,
		WorkflowName: "Webhook Notifier",
		RetryCount:   1,
		Data:         buildDeadLetterData(t, km),
	}
	items := workflow.NewWorkflowItems([]domain.DeadLetter{letter})

	ctx := context.WithValue(context.Background(), workflow.WorkflowContextKey, &workflow.WorkflowContext{
		Id: "test", Name: "dead-letter-replay", Attr: map[string]any{},
	})
	_, err := d.Process(ctx, items)
	assert.NoError(t, err)
	assert.Equal(t, 1, proc.calls)
}

func TestDispatcher_CachesWorkflowRoot(t *testing.T) {
	proc := &stubProcessor{}
	d := makeDispatcher(nil)
	d.roots["Webhook Notifier"] = proc

	km := workflow.KafkaMessage{Topic: "t", Key: "k", Value: json.RawMessage(`{"payload":{"op":"c","tenant_id":"t1","subscriber":{},"occurred_at":"2024-01-01T00:00:00Z","event_id":"e1"}}`)}
	mkLetter := func(id int64) workflow.WorkflowItem {
		l := domain.DeadLetter{ID: id, WorkflowName: "Webhook Notifier", Data: buildDeadLetterData(t, km)}
		return workflow.WorkflowItem{Payload: l}
	}

	ctx := context.WithValue(context.Background(), workflow.WorkflowContextKey, &workflow.WorkflowContext{
		Id: "test", Name: "dead-letter-replay", Attr: map[string]any{},
	})
	_, err := d.Process(ctx, []workflow.WorkflowItem{mkLetter(1), mkLetter(2)})
	assert.NoError(t, err)
	// proc called once per letter (2 letters)
	assert.Equal(t, 2, proc.calls)
}

func TestDispatcher_ReplayContextContainsLetterIdAndRetryCount(t *testing.T) {
	var capturedCtx context.Context
	proc := &stubProcessor{}
	proc.returnItems = nil
	proc.returnErr = nil

	capturingProc := &capturingProcessor{next: proc, capture: func(ctx context.Context) { capturedCtx = ctx }}

	d := makeDispatcher(nil)
	d.roots["Webhook Notifier"] = capturingProc

	km := workflow.KafkaMessage{Topic: "t", Key: "k", Value: json.RawMessage(`{"payload":{"op":"c","tenant_id":"t1","subscriber":{},"occurred_at":"2024-01-01T00:00:00Z","event_id":"e1"}}`)}
	letter := domain.DeadLetter{ID: 99, RetryCount: 3, WorkflowName: "Webhook Notifier", Data: buildDeadLetterData(t, km)}

	ctx := context.WithValue(context.Background(), workflow.WorkflowContextKey, &workflow.WorkflowContext{
		Id: "test", Name: "dead-letter-replay", Attr: map[string]any{},
	})
	_, err := d.Process(ctx, workflow.NewWorkflowItems([]domain.DeadLetter{letter}))
	require.NoError(t, err)

	wfCtx := workflow.GetWorkflowContext(capturedCtx)
	require.NotNil(t, wfCtx)
	assert.Equal(t, int64(99), wfCtx.Attr[util.ReplayLetterId])
	assert.Equal(t, 3, wfCtx.Attr[util.ReplayRetryCount])
}

type capturingProcessor struct {
	next    workflow.Processor
	capture func(context.Context)
}

func (c *capturingProcessor) SetNext(p workflow.Processor) { c.next = p }
func (c *capturingProcessor) Process(ctx context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	c.capture(ctx)
	return items, nil
}
