package event_streaming

import (
	"context"
	"errors"

	"github.com/gleo/subscribers/internal/application/event-streaming/util"
	"github.com/gleo/subscribers/pkg/partner"
	"github.com/gleo/subscribers/pkg/workflow"
)

type Notifier struct {
	client partner.Client
}

func NewNotifier(client partner.Client) *Notifier {
	return &Notifier{client: client}
}

func (n *Notifier) SetNext(_ workflow.Processor) {
	panic("not allowed")
}

func (n *Notifier) Process(ctx context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	events, err := extractNotifyEvents(items)
	if err != nil {
		return nil, err
	}

	for _, event := range events {
		errProcess := n.processEvent(ctx, event)
		if errProcess != nil {
			return nil, errProcess
		}
	}
	return nil, nil
}

func (n *Notifier) processEvent(ctx context.Context, event util.NotifyEvent) error {
	if !event.Value.Payload.Op.IsCrud() {
		return nil
	}

	switch event.Value.Payload.Op {
	case util.CreateOp:
		return n.processCreate(ctx, event)
	case util.UpdateOp:
		return n.processUpdate(ctx, event)
	case util.DeleteOp:
		return n.processDelete(ctx, event)
	case util.AddToSegmentOp:
		return n.processAddToSegment(ctx, event)
	default:
		return nil
	}
}

func (n *Notifier) processCreate(ctx context.Context, event util.NotifyEvent) error {
	return n.client.Notify(ctx, event.Value.Payload)
}

func (n *Notifier) processUpdate(ctx context.Context, event util.NotifyEvent) error {
	return n.client.Notify(ctx, event.Value.Payload)
}

func (n *Notifier) processDelete(ctx context.Context, event util.NotifyEvent) error {
	return n.client.Notify(ctx, event.Value.Payload)
}

func (n *Notifier) processAddToSegment(ctx context.Context, event util.NotifyEvent) error {
	return n.client.Notify(ctx, event.Value.Payload)
}

func extractNotifyEvents(items []workflow.WorkflowItem) ([]util.NotifyEvent, error) {
	if len(items) == 0 {
		return nil, nil
	}
	blocks := make([]util.NotifyEvent, len(items))
	for i, item := range items {
		block, ok := item.Payload.(util.NotifyEvent)
		if !ok {
			return nil, errors.New("item payload is not util.NotifyEvent")
		}
		blocks[i] = block
	}
	return blocks, nil
}
