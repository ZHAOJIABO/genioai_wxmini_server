package dao

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"va_visionai_server/internal/model"
)

type TaskChainDao struct {
	db *gorm.DB
}

func NewTaskChainDao(db *gorm.DB) *TaskChainDao { return &TaskChainDao{db: db} }

func (d *TaskChainDao) Create(ctx context.Context, tx *gorm.DB, chain *model.TaskChain) error {
	return errors.Wrap(tx.WithContext(ctx).Create(chain).Error, "create task chain")
}

func (d *TaskChainDao) GetByRootTask(ctx context.Context, rootTaskID string) (*model.TaskChain, error) {
	var c model.TaskChain
	if err := d.db.WithContext(ctx).Where("root_task_id = ?", rootTaskID).First(&c).Error; err != nil {
		return nil, errors.Wrap(err, "get task chain by root task")
	}
	return &c, nil
}

func (d *TaskChainDao) Update(ctx context.Context, chain *model.TaskChain) error {
	return errors.Wrap(d.db.WithContext(ctx).Save(chain).Error, "update task chain")
}

func (d *TaskChainDao) GetByNextTask(ctx context.Context, nextTaskID string) (*model.TaskChain, error) {
	var c model.TaskChain
	if err := d.db.WithContext(ctx).Where("next_task_id = ?", nextTaskID).First(&c).Error; err != nil {
		return nil, errors.Wrap(err, "get task chain by next task")
	}
	return &c, nil
}
