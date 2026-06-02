package dead_letter_replay

import (
	"context"
	"io"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/internal/port/persistent"
	"github.com/gleo/subscribers/pkg/workflow"
)

type DeadLetterSource struct {
	config ReplayConfig
	repo   persistent.DeadLetterRepository
	reader cursorReader
}

func NewDeadLetterSource(config ReplayConfig, repo persistent.DeadLetterRepository) workflow.EventSource[domain.DeadLetter] {
	return &DeadLetterSource{
		reader: newReader(config, repo),
	}
}

func newReader(config ReplayConfig, repo persistent.DeadLetterRepository) cursorReader {
	if len(config.Ids) != 0 {
		return newIdsReader(config.Ids, repo)
	}
	if config.TenantId != "" {
		return newTenantReader(config.TenantId, repo)
	}
	return newRangeReader(repo, config.FromId, config.ToId)
}

func (d *DeadLetterSource) Fetch(ctx context.Context) ([]domain.DeadLetter, error) {
	return d.reader.Read(ctx)
}

func (d *DeadLetterSource) Close(_ context.Context) error {
	return nil
}

func (d *DeadLetterSource) Commit(_ context.Context, _ []domain.DeadLetter) error {
	return nil
}

type cursorReader interface {
	Read(ctx context.Context) ([]domain.DeadLetter, error)
}

type idsReader struct {
	repo persistent.DeadLetterRepository
	ids  []int64
}

func newIdsReader(ids []int64, repo persistent.DeadLetterRepository) *idsReader {
	return &idsReader{
		repo: repo,
		ids:  ids,
	}
}

func (i *idsReader) Read(ctx context.Context) ([]domain.DeadLetter, error) {
	if len(i.ids) == 0 {
		return nil, io.EOF
	}
	dls, err := i.repo.GetByIds(ctx, i.ids)
	i.ids = nil
	return dls, err
}

type rangeReader struct {
	repo   persistent.DeadLetterRepository
	fromId int64
	toId   int64
}

func newRangeReader(repo persistent.DeadLetterRepository, fromId, toId int64) *rangeReader {
	return &rangeReader{
		repo:   repo,
		fromId: fromId,
		toId:   toId,
	}
}

func (r *rangeReader) Read(ctx context.Context) ([]domain.DeadLetter, error) {
	if r.toId < r.fromId {
		return nil, io.EOF
	}
	from := r.fromId
	if from < r.toId-100 {
		from = r.toId - 100
	}

	log.Logger.Infof("read dead letter from %d, to %d", from, r.toId)
	dls, err := r.repo.GetByRange(ctx, from, r.toId)
	if err == nil {
		r.toId = from - 1
	}
	return dls, err
}

type tenantReader struct {
	repo      persistent.DeadLetterRepository
	tenantId  string
	currentId int64
}

func newTenantReader(tenantId string, repo persistent.DeadLetterRepository) *tenantReader {
	return &tenantReader{
		repo:     repo,
		tenantId: tenantId,
	}
}

func (r *tenantReader) Read(ctx context.Context) ([]domain.DeadLetter, error) {
	log.Logger.Infof("read dead letter by tenant '%s' size 100", r.tenantId)
	dls, err := r.repo.GetByTenantId(ctx, r.tenantId, 100, r.currentId)
	if err != nil {
		return nil, err
	}
	if len(dls) == 0 {
		r.currentId = -1
		return nil, io.EOF
	}
	r.currentId = dls[len(dls)-1].ID
	return dls, nil
}

type workflowReader struct {
	repo         persistent.DeadLetterRepository
	workflowName string
	currentId    int64
}

func newWorkflowReader(processorName string, repo persistent.DeadLetterRepository) *workflowReader {
	return &workflowReader{
		repo:         repo,
		workflowName: processorName,
	}
}

func (r *workflowReader) Read(ctx context.Context) ([]domain.DeadLetter, error) {
	log.Logger.Infof("read dead letter from by workflow '%s' size 100", r.workflowName)
	dls, err := r.repo.GetByWorkflow(ctx, r.workflowName, 100, r.currentId)
	if err != nil {
		return nil, err
	}
	if len(dls) == 0 {
		r.currentId = -1
		return nil, io.EOF
	}
	r.currentId = dls[len(dls)-1].ID
	return dls, nil
}
