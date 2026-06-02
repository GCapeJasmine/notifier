package persistent

import (
	"context"

	"github.com/gleo/subscribers/internal/domain"
)

type DeadLetterRepository interface {
	GetByIds(ctx context.Context, ids []int64) ([]domain.DeadLetter, error)
	GetByRange(ctx context.Context, fromId int64, toId int64) ([]domain.DeadLetter, error)
	GetByTenantId(ctx context.Context, tenantId string, limit int, upperId int64) ([]domain.DeadLetter, error)
	GetByWorkflow(ctx context.Context, name string, limit int, upperId int64) ([]domain.DeadLetter, error)

	Insert(ctx context.Context, deadLetter domain.DeadLetter) error
	UpdateError(ctx context.Context, id int64, retryCount int, err error) error
	Delete(ctx context.Context, ids []int64) error
}
