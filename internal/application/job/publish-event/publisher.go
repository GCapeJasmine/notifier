package publish_event

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	kafkapkg "github.com/gleo/subscribers/common/kafka"
	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/internal/application/event-streaming/util"
	kafka "github.com/segmentio/kafka-go"
)

var allOps = []util.EventOp{
	util.CreateOp,
	util.UpdateOp,
	util.DeleteOp,
	util.AddToSegmentOp,
}

type Publisher struct {
	writer kafkapkg.Writer
	config PublishEventConfig
}

func NewPublisher(cfg PublishEventConfig) *Publisher {
	return &Publisher{
		writer: kafkapkg.NewWriter(cfg.KafkaWriter),
		config: cfg,
	}
}

func (p *Publisher) Run(ctx context.Context) error {
	defer p.writer.Close()

	for {
		for _, tenantId := range p.config.Event.TenantIds {
			for _, sub := range p.config.Event.Subscribers {
				for _, op := range allOps {
					if ctx.Err() != nil {
						return nil
					}
					p.writeWithRetry(ctx, tenantId, sub, op)
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(p.config.Event.Interval):
		}
	}
}

func (p *Publisher) writeWithRetry(ctx context.Context, tenantId string, sub SubscriberConfig, op util.EventOp) {
	msg, err := p.buildMessage(tenantId, sub, op)
	if err != nil {
		log.Logger.Errorw("publish-event: failed to build message", "op", op, zap.Error(err))
		return
	}

	for {
		if ctx.Err() != nil {
			return
		}
		if err := p.writer.WriteMessage(ctx, msg); err != nil {
			log.Logger.Warnw("publish-event: write failed, retrying",
				"tenant_id", tenantId,
				"subscriber_id", sub.SubscriberId,
				"op", op,
				zap.Error(err),
			)
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		log.Logger.Infow("publish-event: published",
			"tenant_id", tenantId,
			"subscriber_id", sub.SubscriberId,
			"op", op,
		)
		return
	}
}

func (p *Publisher) buildMessage(tenantId string, sub SubscriberConfig, op util.EventOp) (kafka.Message, error) {
	keyBytes, err := json.Marshal(util.NotifyEventKey{
		TenantId:     tenantId,
		SubscriberId: sub.SubscriberId,
	})
	if err != nil {
		return kafka.Message{}, err
	}

	valueBytes, err := json.Marshal(util.NotifyEventValue{
		Payload: util.NotifyEventPayload{
			EventId:    uuid.NewString(),
			Op:         op,
			TenantId:   tenantId,
			Subscriber: json.RawMessage(sub.Data),
			OccurredAt: time.Now(),
		},
	})
	if err != nil {
		return kafka.Message{}, err
	}

	return kafka.Message{
		Key:   keyBytes,
		Value: valueBytes,
	}, nil
}
