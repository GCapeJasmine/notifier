package deadletter

import (
	"context"
	"time"

	"github.com/jackc/pgtype"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/gleo/subscribers/common/log"
	"github.com/gleo/subscribers/internal/domain"
	"github.com/gleo/subscribers/internal/port/persistent"
)

type deadLetter struct {
	ID           int64        `gorm:"column:id;type:bigserial;primarykey"`
	TenantID     string       `gorm:"column:tenant_id;index"`
	WorkflowName string       `gorm:"column:workflow_name;index"`
	Error        string       `gorm:"column:error;type:text"`
	Data         pgtype.JSONB `gorm:"column:data;type:jsonb"`
	RetryCount   int          `gorm:"column:retry_count;default:1"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type deadLetterRepository struct {
	db *gorm.DB
}

func NewDeadLetterRepository(db *gorm.DB) (persistent.DeadLetterRepository, error) {
	if err := db.AutoMigrate(&deadLetter{}); err != nil {
		return nil, err
	}
	return &deadLetterRepository{db: db}, nil
}

func (d *deadLetterRepository) GetByIds(ctx context.Context, ids []int64) ([]domain.DeadLetter, error) {
	var deadLetters []deadLetter
	err := d.db.WithContext(ctx).Find(&deadLetters, ids).Error
	if err != nil {
		return nil, err
	}
	dls := newDomainDeadLetters(deadLetters)
	return dls, nil
}

func (d *deadLetterRepository) GetByRange(ctx context.Context, fromId int64, toId int64) ([]domain.DeadLetter, error) {
	var deadLetters []deadLetter
	err := d.db.WithContext(ctx).Where("id between ? and ?", fromId, toId).Find(&deadLetters).Error
	if err != nil {
		return nil, err
	}
	dls := newDomainDeadLetters(deadLetters)
	return dls, nil
}

func (d *deadLetterRepository) GetByTenantId(ctx context.Context, tenantId string, limit int, upperId int64) ([]domain.DeadLetter, error) {
	var deadLetters []deadLetter
	query := d.db.WithContext(ctx).Limit(limit).Order("id desc")
	if upperId == 0 {
		query = query.Where("tenant_id = ?", tenantId)
	} else {
		query = query.Where("tenant_id = ? and id < ?", tenantId, upperId)
	}
	if err := query.Find(&deadLetters).Error; err != nil {
		return nil, err
	}
	return newDomainDeadLetters(deadLetters), nil
}

func (d *deadLetterRepository) GetByWorkflow(ctx context.Context, name string, limit int, upperId int64) ([]domain.DeadLetter, error) {
	var deadLetters []deadLetter
	query := d.db.WithContext(ctx).Limit(limit).Order("id desc")
	if upperId == 0 {
		query = query.Where("workflow_name = ?", name)
	} else {
		query = query.Where("workflow_name = ? and id < ?", name, upperId)
	}

	if err := query.Find(&deadLetters).Error; err != nil {
		return nil, err
	}

	return newDomainDeadLetters(deadLetters), nil
}

func (d *deadLetterRepository) Insert(ctx context.Context, deadLetter domain.DeadLetter) error {
	dl := newDeadLetter(deadLetter)
	return d.db.WithContext(ctx).Create(&dl).Error
}

func (d *deadLetterRepository) UpdateError(ctx context.Context, id int64, retryCount int, err error) error {
	letter := deadLetter{ID: id}
	return d.db.WithContext(ctx).Model(&letter).
		Updates(deadLetter{
			RetryCount: retryCount,
			Error:      err.Error(),
			UpdatedAt:  time.Now(),
		}).
		Error
}

func (d *deadLetterRepository) Delete(ctx context.Context, ids []int64) error {
	var deadLetters []deadLetter
	return d.db.WithContext(ctx).Delete(&deadLetters, ids).Error
}

func newDomainDeadLetters(dl []deadLetter) []domain.DeadLetter {
	if len(dl) == 0 {
		return nil
	}

	results := make([]domain.DeadLetter, len(dl))
	for i, _ := range dl {
		results[i] = dl[i].toDomain()
	}
	return results
}

func newDeadLetter(dl domain.DeadLetter) deadLetter {
	return deadLetter{
		TenantID:     dl.TenantId,
		WorkflowName: dl.WorkflowName,
		Error:        dl.Error,
		RetryCount:   0,
		Data:         anyAsJsonb(dl.Data),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
}

func anyAsJsonb(v any) pgtype.JSONB {
	jsonb := pgtype.JSONB{}
	err := jsonb.Set(v)
	if err != nil {
		log.Logger.Errorw("cannot set JsonB field", zap.Error(err))
	}
	return jsonb
}

func (d deadLetter) toDomain() domain.DeadLetter {
	return domain.DeadLetter{
		ID:           d.ID,
		TenantId:     d.TenantID,
		WorkflowName: d.WorkflowName,
		RetryCount:   d.RetryCount,
		Data:         d.Data.Bytes,
	}
}

func (d deadLetter) TableName() string {
	return "dead_letters"
}
