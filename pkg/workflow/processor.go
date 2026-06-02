package workflow

import (
	"context"
	"time"

	"github.com/gleo/subscribers/common/utils"
)

type Processor interface {
	SetNext(Processor)
	Process(ctx context.Context, items []WorkflowItem) ([]WorkflowItem, error)
}

type Retry struct {
	retryOption utils.RetryOption
	next        Processor
}

func NewRetry(retryOption utils.RetryOption) *Retry {
	return &Retry{
		retryOption: retryOption,
	}
}

func (r *Retry) Process(ctx context.Context, items []WorkflowItem) ([]WorkflowItem, error) {
	return utils.ProcessWithDataRetryOption(func() ([]WorkflowItem, error) {
		return r.next.Process(ctx, items)
	}, r.retryOption)
}

func (r *Retry) SetNext(processor Processor) {
	r.next = processor
}

type MaxWaitDuration time.Duration

func (m *MaxWaitDuration) UnmarshalConfigItem(value string) error {
	duration, err := time.ParseDuration(value)
	if err != nil {
		return err
	}

	*m = MaxWaitDuration(duration)
	return nil
}

type MaxWait struct {
	maxWait MaxWaitDuration
	next    Processor
}

func NewMaxWait(maxWait MaxWaitDuration) *MaxWait {
	return &MaxWait{maxWait: maxWait}
}

func (r *MaxWait) Process(ctx context.Context, items []WorkflowItem) ([]WorkflowItem, error) {
	if r.maxWait == 0 {
		return r.next.Process(ctx, items)
	}

	maxWaitCtx, cancelFn := context.WithTimeout(ctx, time.Duration(r.maxWait))
	defer cancelFn()

	return r.next.Process(maxWaitCtx, items)
}

func (r *MaxWait) SetNext(processor Processor) {
	r.next = processor
}

