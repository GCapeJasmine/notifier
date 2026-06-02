package dead_letter_replay

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/gleo/subscribers/internal/domain"
)

func makeDeadLetters(ids ...int64) []domain.DeadLetter {
	out := make([]domain.DeadLetter, len(ids))
	for i, id := range ids {
		out[i] = domain.DeadLetter{ID: id, TenantId: "t1", WorkflowName: "wf"}
	}
	return out
}

// idsReader

func TestIdsReader_EmptyIds_EOF(t *testing.T) {
	r := newIdsReader(nil, &stubRepo{})
	_, err := r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestIdsReader_ReturnsRecordsThenEOF(t *testing.T) {
	repo := &stubRepo{byIds: map[int64]domain.DeadLetter{1: {ID: 1}, 2: {ID: 2}}}
	r := newIdsReader([]int64{1, 2}, repo)

	dls, err := r.Read(context.Background())
	assert.NoError(t, err)
	assert.Len(t, dls, 2)

	_, err = r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

// tenantReader

func TestTenantReader_EmptyResult_EOF(t *testing.T) {
	r := newTenantReader("t1", &stubRepo{})
	_, err := r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestTenantReader_PaginatesAndExits(t *testing.T) {
	// 3 records ordered descending by ID
	repo := &stubRepo{byTenant: makeDeadLetters(3, 2, 1)}
	r := newTenantReader("t1", repo)

	// First page: all 3 records (limit 100, upperId 0 → no filter)
	dls, err := r.Read(context.Background())
	assert.NoError(t, err)
	assert.Len(t, dls, 3)
	assert.Equal(t, int64(1), r.currentId) // last record's ID

	// Second page: upperId=1, stub returns nothing for id<1
	_, err = r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

// rangeReader

func TestRangeReader_ToIdBelowFromId_EOF(t *testing.T) {
	r := newRangeReader(&stubRepo{}, 5, 3) // toId < fromId
	_, err := r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestRangeReader_AdvancesCursorAndExits(t *testing.T) {
	repo := &stubRepo{byRange: makeDeadLetters(5, 4, 3)}
	r := newRangeReader(repo, 1, 5)

	dls, err := r.Read(context.Background())
	assert.NoError(t, err)
	assert.Len(t, dls, 3)
	// toId advanced to from-1; next call: toId(from-1) < fromId → EOF
	_, err = r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

// workflowReader

func TestWorkflowReader_ExitsWhenEmpty(t *testing.T) {
	r := newWorkflowReader("wf", &stubRepo{})
	_, err := r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}

func TestWorkflowReader_CursorAdvancesEachPage(t *testing.T) {
	repo := &stubRepo{byWorkflow: makeDeadLetters(10, 9, 8)}
	r := newWorkflowReader("wf", repo)

	dls, err := r.Read(context.Background())
	assert.NoError(t, err)
	assert.Len(t, dls, 3)
	assert.Equal(t, int64(8), r.currentId) // last record ID

	// Next page empty → EOF
	_, err = r.Read(context.Background())
	assert.ErrorIs(t, err, io.EOF)
}
