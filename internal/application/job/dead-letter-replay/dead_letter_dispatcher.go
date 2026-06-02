package dead_letter_replay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/multierr"

	"github.com/gleo/subscribers/internal/application/event-streaming/config"
	"github.com/gleo/subscribers/internal/application/event-streaming/util"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/internal/port/persistent"
	"github.com/gleo/subscribers/pkg/workflow"
)

type WorkflowSupplier func(config config.EventStreamingConfig) (*workflow.Workflow[workflow.KafkaMessage], error)

type itemKafkaMessage struct {
	Payload workflow.KafkaMessage `json:"payload"`
}

type DeadLetterDispatcher struct {
	streamConfig      config.EventStreamingConfig
	repo              persistent.DeadLetterRepository
	workflowSuppliers map[string]WorkflowSupplier
	roots             map[string]workflow.Processor
}

func NewDeadLetterDispatcher(
	repo persistent.DeadLetterRepository,
	workflowSuppliers map[string]WorkflowSupplier,
	streamConfig config.EventStreamingConfig,
) *DeadLetterDispatcher {

	return &DeadLetterDispatcher{
		repo:              repo,
		streamConfig:      streamConfig,
		workflowSuppliers: workflowSuppliers,
		roots:             make(map[string]workflow.Processor),
	}
}

func (d *DeadLetterDispatcher) SetNext(_ workflow.Processor) {
	panic("not allowed")
}

func (d *DeadLetterDispatcher) Process(ctx context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	if len(items) == 0 {
		return nil, nil
	}

	for i := range items {
		workflowCtx := workflow.GetWorkflowContext(ctx)
		letter, err := d.parse(items[i])
		if err != nil {
			return nil, err
		}
		root, err := d.getWorkflow(letter.WorkflowName)
		if err != nil {
			return nil, err
		}
		targetItems, err := d.parseToDownStream(letter)
		if err != nil {
			return nil, err
		}
		// change name into the original workflow, setup util.DeadLetterProcessor ignore flag
		workflowCtx.Name = letter.WorkflowName
		workflowCtx.Attr[util.ReplayLetterId] = letter.ID
		workflowCtx.Attr[util.ReplayRetryCount] = letter.RetryCount

		_, err = root.Process(ctx, targetItems)
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (d *DeadLetterDispatcher) parseToDownStream(letter domain.DeadLetter) ([]workflow.WorkflowItem, error) {
	var items []itemKafkaMessage
	err := json.Unmarshal(letter.Data, &items)
	if err != nil {
		return nil, err
	}

	results := make([]workflow.WorkflowItem, len(items))
	for i, item := range items {
		results[i] = workflow.WorkflowItem{
			Payload: item.Payload,
		}
	}
	return results, err
}

func (d *DeadLetterDispatcher) parse(item workflow.WorkflowItem) (domain.DeadLetter, error) {
	letter, ok := item.Payload.(domain.DeadLetter)
	if !ok {
		return domain.DeadLetter{}, errors.New("item payload is not DeadLetter")
	}
	return letter, nil
}

func (d *DeadLetterDispatcher) getWorkflow(workflowName string) (workflow.Processor, error) {
	root := d.roots[workflowName]
	if root != nil {
		return root, nil
	}
	sup, ok := d.workflowSuppliers[workflowName]
	if !ok {
		return nil, fmt.Errorf("there is no workflow of '%s'", workflowName)
	}
	wf, err := sup(d.streamConfig)
	if err != nil {
		return nil, multierr.Append(errors.New("cannot initial new workflow"), err)
	}

	root = wf.GetRoot()
	d.roots[workflowName] = root
	return root, nil
}
