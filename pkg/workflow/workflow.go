package workflow

import (
	"context"
	"errors"
	"io"

	"github.com/google/uuid"
	"go.uber.org/multierr"
	"go.uber.org/zap"

	"github.com/gleo/subscribers/common/log"
)

const (
	WorkflowContextKey = "workflow_ctx"
)

type WorkflowItem struct {
	Payload any `json:"payload"`
}

func NewWorkflowItems[T any](payloads []T) []WorkflowItem {
	if len(payloads) == 0 {
		return nil
	}

	results := make([]WorkflowItem, len(payloads))
	for i, item := range payloads {
		results[i] = WorkflowItem{Payload: item}
	}
	return results
}

type WorkflowBuilder[T any] struct {
	source     EventSource[T]
	processors []Processor
	name       string
}

func NewWorkflowBuilder[T any]() *WorkflowBuilder[T] {
	return &WorkflowBuilder[T]{
		processors: make([]Processor, 0),
	}
}

func (w *WorkflowBuilder[T]) Name(name string) *WorkflowBuilder[T] {
	w.name = name
	return w
}

func (w *WorkflowBuilder[T]) Source(source EventSource[T]) *WorkflowBuilder[T] {
	w.source = source
	return w
}

func (w *WorkflowBuilder[T]) AddProcessor(processor Processor) *WorkflowBuilder[T] {
	if processor == nil {
		panic("added processor should not be nil")
	}

	w.processors = append(w.processors, processor)
	return w
}

func (w *WorkflowBuilder[T]) Build() (*Workflow[T], error) {
	if len(w.processors) == 0 {
		return nil, errors.New("workflow is empty")
	}

	// chain processors
	processors := w.getProcessors()
	for i := 0; i < len(processors)-1; i++ {
		processors[i].SetNext(processors[i+1])
	}

	return &Workflow[T]{
		name:   w.name,
		source: w.source,
		root:   processors[0],
	}, nil
}

func (w *WorkflowBuilder[T]) getProcessors() []Processor {
	return w.processors
}

type Workflow[T any] struct {
	source EventSource[T]
	root   Processor
	name   string
}

func (w *Workflow[T]) Run(ctx context.Context) (valCtx context.Context, errRun error) {
	defer func() {
		err := w.source.Close(ctx)
		if err != nil {
			log.Logger.Errorw("cannot close source", zap.Error(err))
		}

		if errRun != nil {
			workflowCtx := GetWorkflowContext(valCtx)
			log.Logger.Errorw("cannot process workflow", "workflowId", workflowCtx.Id, "workflowName", workflowCtx.Name, zap.Error(errRun))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx, nil
		default:
			workflowCtx := context.WithValue(ctx, WorkflowContextKey, newWorkflowContext(w.name))
			err := w.process(workflowCtx)
			if err != nil {
				// graceful shutdown thread once EOF
				if errors.Is(err, io.EOF) {
					log.Logger.Infow("reached to the end of the source, stop the workflow")
					return workflowCtx, nil
				}

				// unexpected errors, fail fast
				return workflowCtx, multierr.Append(errors.New("workflow got unexpected error, stop to process"), err)
			}
		}
	}
}

func (w *Workflow[T]) process(ctx context.Context) error {
	events, err := w.source.Fetch(ctx)

	switch {
	case errors.Is(err, io.EOF):
		return err
	case err != nil:
		log.Logger.Errorw("cannot fetch from source", zap.Error(err))
		return nil
	}

	_, err = w.root.Process(ctx, NewWorkflowItems(events))
	if err != nil {
		return multierr.Append(errors.New("process got error"), err)
	}

	err = w.source.Commit(ctx, events)
	if err != nil {
		log.Logger.Errorw("cannot commit messages from source workflow will ignore this error and continue to process", zap.Error(err))
	}
	return nil
}

// GetRoot
// this is quite a bit dirty, to get root processors and assemble into the dead_letter_replay.DeadLetterDispatcher
func (w *Workflow[T]) GetRoot() Processor {
	return w.root
}

func GetWorkflowContext(ctx context.Context) *WorkflowContext {
	pCtx, _ := ctx.Value(WorkflowContextKey).(*WorkflowContext)
	return pCtx
}

type WorkflowContext struct {
	Id   string
	Name string
	Attr map[string]any
}

func newWorkflowContext(name string) *WorkflowContext {
	return &WorkflowContext{
		Id:   uuid.NewString(),
		Name: name,
		Attr: make(map[string]any),
	}
}
