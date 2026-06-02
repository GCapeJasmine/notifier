package util

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/internal/port/persistent"
	"github.com/gleo/subscribers/pkg/workflow"
)

type DeadLetterMessage struct {
	WorkflowName string          `json:"workflow_name"`
	WorkflowID   string          `json:"workflow_id"`
	Error        string          `json:"error"`
	Payload      json.RawMessage `json:"payload"`
}

const (
	ReplayRetryCount = "replayRetryCount"
	ReplayLetterId   = "letterId"
)

type DeadLetterProcessor struct {
	deadLetterRepo persistent.DeadLetterRepository
	next           workflow.Processor
}

func NewDeadLetterProcessor(deadLetterRepo persistent.DeadLetterRepository) *DeadLetterProcessor {
	return &DeadLetterProcessor{deadLetterRepo: deadLetterRepo}
}

func (d *DeadLetterProcessor) SetNext(next workflow.Processor) {
	d.next = next
}

func (d *DeadLetterProcessor) Process(ctx context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	workflowCtx := workflow.GetWorkflowContext(ctx)
	results, err := d.processNext(ctx, items)

	_, isReplaying := workflowCtx.Attr[ReplayLetterId]
	if isReplaying {
		d.handleRetryResult(ctx, err)
		return results, nil
	}

	if err != nil {
		log.Logger.Errorw("processing items got error, move items to DeadLettersStore", "workflowId", workflowCtx.Id, "workflowName", workflowCtx.Name, zap.Error(err))

		letter, err := d.toDeadLetter(workflowCtx.Name, err, items)
		if err != nil {
			return nil, multierr.Append(errors.New("cannot parse workflow items to dead letter"), err)
		}

		err = d.deadLetterRepo.Insert(ctx, letter)
		if err != nil {
			// propagate error, expecting pod restarts to avoid losing messages
			return nil, multierr.Append(errors.New("cannot save to DeadLetter"), err)
		}
		log.Logger.Infow("dead letter stored", "items", items, "workflow", workflowCtx.Name, "id", workflowCtx.Id)

		// return back the items, not results (probably nil)
		return items, nil
	}

	return results, nil
}

func (d *DeadLetterProcessor) processNext(ctx context.Context, items []workflow.WorkflowItem) (results []workflow.WorkflowItem, err error) {
	defer func() {
		if pa := recover(); pa != nil {
			switch t := pa.(type) {
			case string:
				err = errors.New(t)
			case error:
				err = t
			default:
				err = fmt.Errorf("got unknown panic, %v", pa)
			}
		}
	}()

	results, err = d.next.Process(ctx, items)
	return
}

func (d *DeadLetterProcessor) handleRetryResult(ctx context.Context, err error) {
	workflowCtx := workflow.GetWorkflowContext(ctx)
	letterIdRaw, _ := workflowCtx.Attr[ReplayLetterId]
	letterId := letterIdRaw.(int64)
	retryCountRaw, _ := workflowCtx.Attr[ReplayRetryCount]
	retryCount := retryCountRaw.(int)

	if err != nil {
		log.Logger.Infow("replay unsuccessful", "id", letterId, zap.Error(err))
		if err := d.deadLetterRepo.UpdateError(ctx, letterId, retryCount+1, err); err != nil {
			log.Logger.Errorw("cannot update error to dead letter", "id", letterId, zap.Error(err))
		}
		return
	}
	if err := d.deadLetterRepo.Delete(ctx, []int64{letterId}); err != nil {
		log.Logger.Errorw("cannot update error to dead letter", "id", letterId, zap.Error(err))
	}
	log.Logger.Infow("replay successful", "id", letterId)
}

func (d *DeadLetterProcessor) toDeadLetter(workflowName string, err error, items []workflow.WorkflowItem) (domain.DeadLetter, error) {
	if len(items) == 0 {
		return domain.DeadLetter{}, errors.New("found empty items, nothing will be save in dead letter")
	}

	bytes, errM := json.Marshal(items)
	if errM != nil {
		return domain.DeadLetter{}, errM
	}

	return domain.DeadLetter{
		WorkflowName: workflowName,
		TenantId:     extractTenantId(items),
		Data:         bytes,
		Error:        err.Error(),
	}, nil
}

func extractTenantId(items []workflow.WorkflowItem) string {
	if len(items) == 0 {
		return ""
	}
	km, ok := items[0].Payload.(workflow.KafkaMessage)
	if !ok {
		return ""
	}
	var key struct {
		TenantId string `json:"tenant_id"`
	}
	_ = json.Unmarshal([]byte(km.Key), &key)
	return key.TenantId
}
