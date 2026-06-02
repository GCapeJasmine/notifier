package dead_letter_replay

import (
	"context"
	"io"

	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/pkg/workflow"
)

// --- DeadLetterRepository stub ---

type stubRepo struct {
	byIds      map[int64]domain.DeadLetter
	byTenant   []domain.DeadLetter
	byWorkflow []domain.DeadLetter
	byRange    []domain.DeadLetter
	insertErr  error
	updateErr  error
	deleteErr  error
	inserted   []domain.DeadLetter
	deleted    []int64
	updated    []int64
}

func (s *stubRepo) GetByIds(_ context.Context, ids []int64) ([]domain.DeadLetter, error) {
	var out []domain.DeadLetter
	for _, id := range ids {
		if dl, ok := s.byIds[id]; ok {
			out = append(out, dl)
		}
	}
	return out, nil
}

func (s *stubRepo) GetByRange(_ context.Context, _, _ int64) ([]domain.DeadLetter, error) {
	return s.byRange, nil
}

func (s *stubRepo) GetByTenantId(_ context.Context, _ string, limit int, upperId int64) ([]domain.DeadLetter, error) {
	var out []domain.DeadLetter
	for _, dl := range s.byTenant {
		if upperId != 0 && dl.ID >= upperId {
			continue
		}
		out = append(out, dl)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *stubRepo) GetByWorkflow(_ context.Context, _ string, limit int, upperId int64) ([]domain.DeadLetter, error) {
	var out []domain.DeadLetter
	for _, dl := range s.byWorkflow {
		if upperId != 0 && dl.ID >= upperId {
			continue
		}
		out = append(out, dl)
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *stubRepo) Insert(_ context.Context, dl domain.DeadLetter) error {
	s.inserted = append(s.inserted, dl)
	return s.insertErr
}

func (s *stubRepo) UpdateError(_ context.Context, id int64, _ int, _ error) error {
	s.updated = append(s.updated, id)
	return s.updateErr
}

func (s *stubRepo) Delete(_ context.Context, ids []int64) error {
	s.deleted = append(s.deleted, ids...)
	return s.deleteErr
}

// --- Processor stub ---

type stubProcessor struct {
	returnItems []workflow.WorkflowItem
	returnErr   error
	calls       int
}

func (s *stubProcessor) SetNext(_ workflow.Processor) {}

func (s *stubProcessor) Process(_ context.Context, items []workflow.WorkflowItem) ([]workflow.WorkflowItem, error) {
	s.calls++
	if s.returnItems != nil {
		return s.returnItems, s.returnErr
	}
	return items, s.returnErr
}

// --- EOF source helper ---

type eofSource struct{}

func (e *eofSource) Fetch(_ context.Context) ([]domain.DeadLetter, error) { return nil, io.EOF }
func (e *eofSource) Close(_ context.Context) error                        { return nil }
func (e *eofSource) Commit(_ context.Context, _ []domain.DeadLetter) error { return nil }
