package util

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/pkg/workflow"
)

// stubDeadLetterRepo records Insert / UpdateError / Delete calls.
type stubDeadLetterRepo struct {
	insertErr error
	updateErr error
	deleteErr error
	inserted  []domain.DeadLetter
	updated   []int64
	deleted   []int64
}

func (s *stubDeadLetterRepo) GetByIds(_ context.Context, _ []int64) ([]domain.DeadLetter, error) {
	return nil, nil
}
func (s *stubDeadLetterRepo) GetByRange(_ context.Context, _, _ int64) ([]domain.DeadLetter, error) {
	return nil, nil
}
func (s *stubDeadLetterRepo) GetByTenantId(_ context.Context, _ string, _ int, _ int64) ([]domain.DeadLetter, error) {
	return nil, nil
}
func (s *stubDeadLetterRepo) GetByWorkflow(_ context.Context, _ string, _ int, _ int64) ([]domain.DeadLetter, error) {
	return nil, nil
}
func (s *stubDeadLetterRepo) Insert(_ context.Context, dl domain.DeadLetter) error {
	s.inserted = append(s.inserted, dl)
	return s.insertErr
}
func (s *stubDeadLetterRepo) UpdateError(_ context.Context, id int64, _ int, _ error) error {
	s.updated = append(s.updated, id)
	return s.updateErr
}
func (s *stubDeadLetterRepo) Delete(_ context.Context, ids []int64) error {
	s.deleted = append(s.deleted, ids...)
	return s.deleteErr
}

func makeWfCtx(attrs map[string]any) context.Context {
	return context.WithValue(context.Background(), workflow.WorkflowContextKey, &workflow.WorkflowContext{
		Id:   "test-id",
		Name: "Webhook Notifier",
		Attr: attrs,
	})
}

func makeKafkaItems() []workflow.WorkflowItem {
	km := workflow.KafkaMessage{
		Key:   `{"tenant_id":"t1","subscriber_id":"s1"}`,
		Topic: "subscribers.events",
	}
	return workflow.NewWorkflowItems([]workflow.KafkaMessage{km})
}

func TestDeadLetterProcessor_SuccessPath_PassesThrough(t *testing.T) {
	repo := &stubDeadLetterRepo{}
	next := &stubProcessor{received: nil}
	proc := NewDeadLetterProcessor(repo)
	proc.SetNext(next)

	items := makeKafkaItems()
	result, err := proc.Process(makeWfCtx(map[string]any{}), items)

	assert.NoError(t, err)
	assert.Len(t, result, len(items))
	assert.Empty(t, repo.inserted)
}

func TestDeadLetterProcessor_ErrorPath_SavesToDB(t *testing.T) {
	repo := &stubDeadLetterRepo{}
	next := &stubProcessor{err: errors.New("partner failed")}
	proc := NewDeadLetterProcessor(repo)
	proc.SetNext(next)

	items := makeKafkaItems()
	result, err := proc.Process(makeWfCtx(map[string]any{}), items)

	assert.NoError(t, err) // error is absorbed, not propagated
	assert.Len(t, result, len(items))
	require.Len(t, repo.inserted, 1)
	assert.Equal(t, "partner failed", repo.inserted[0].Error)
	assert.Equal(t, "t1", repo.inserted[0].TenantId)
}

func TestDeadLetterProcessor_ReplayMode_Success_Deletes(t *testing.T) {
	repo := &stubDeadLetterRepo{}
	next := &stubProcessor{} // success
	proc := NewDeadLetterProcessor(repo)
	proc.SetNext(next)

	ctx := makeWfCtx(map[string]any{
		ReplayLetterId:   int64(42),
		ReplayRetryCount: 2,
	})
	_, err := proc.Process(ctx, makeKafkaItems())

	assert.NoError(t, err)
	assert.Contains(t, repo.deleted, int64(42))
	assert.Empty(t, repo.updated)
}

func TestDeadLetterProcessor_ReplayMode_Failure_UpdatesError(t *testing.T) {
	repo := &stubDeadLetterRepo{}
	next := &stubProcessor{err: errors.New("still failing")}
	proc := NewDeadLetterProcessor(repo)
	proc.SetNext(next)

	ctx := makeWfCtx(map[string]any{
		ReplayLetterId:   int64(42),
		ReplayRetryCount: 2,
	})
	_, err := proc.Process(ctx, makeKafkaItems())

	assert.NoError(t, err)
	assert.Contains(t, repo.updated, int64(42))
	assert.Empty(t, repo.deleted)
}

func TestDeadLetterProcessor_PanicRecovery_SavesToDB(t *testing.T) {
	repo := &stubDeadLetterRepo{}
	panicProc := &panicProcessor{}
	proc := NewDeadLetterProcessor(repo)
	proc.SetNext(panicProc)

	items := makeKafkaItems()
	_, err := proc.Process(makeWfCtx(map[string]any{}), items)

	assert.NoError(t, err)
	require.Len(t, repo.inserted, 1)
	assert.Contains(t, repo.inserted[0].Error, "oh no")
}

type panicProcessor struct{}

func (p *panicProcessor) SetNext(_ workflow.Processor) {}
func (p *panicProcessor) Process(_ context.Context, _ []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	panic("oh no")
}
