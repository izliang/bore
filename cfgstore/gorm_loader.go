package cfgstore

import (
	"context"
	"time"

	"github.com/izliang/bore/logit"
	"gorm.io/gorm"
)

// GormLoader 基于 GORM 的配置加载器
type GormLoader struct {
	db *gorm.DB
}

// NewGormLoader 创建基于数据库的配置加载器
func NewGormLoader(db *gorm.DB) *GormLoader {
	l := &GormLoader{
		db: db,
	}
	return l
}

// LoadAll 全量加载所有配置
func (l *GormLoader) LoadAll(ctx context.Context, logger logit.Logger) (LoadResult, error) {
	var records []struct {
		Name       string    `gorm:"column:name"`
		Value      string    `gorm:"column:value"`
		IsDelete   int       `gorm:"column:is_delete"`
		UpdateTime time.Time `gorm:"column:update_time"`
	}

	err := l.db.WithContext(ctx).
		Select("name, value, is_delete, update_time").
		Table("params_config").
		Find(&records).Error

	if err != nil {
		return LoadResult{}, err
	}

	data := make(map[string]string)
	var maxUpdateTime time.Time

	for _, r := range records {
		if r.IsDelete == 1 {
			data[r.Name] = "" // 标记删除
		} else {
			data[r.Name] = r.Value
		}
		if r.UpdateTime.After(maxUpdateTime) {
			maxUpdateTime = r.UpdateTime
		}
	}
	logger.Info(ctx, "load all config", logit.IntField("nums", len(data)), logit.StringField("nextSince", maxUpdateTime.Format(time.RFC3339)))
	logger.Debug(ctx, "load all config detail", logit.IntField("nums", len(data)),
		logit.StringField("nextSince", maxUpdateTime.Format(time.RFC3339)), logit.JSONAbsField("data", data))

	return LoadResult{
		Data:      data,
		NextSince: maxUpdateTime,
	}, nil
}

// LoadSince 增量加载 since 之后变更的配置
func (l *GormLoader) LoadSince(ctx context.Context, logger logit.Logger, since time.Time) (LoadResult, error) {
	var records []struct {
		Name       string    `gorm:"column:name"`
		Value      string    `gorm:"column:value"`
		IsDelete   int       `gorm:"column:is_delete"`
		UpdateTime time.Time `gorm:"column:update_time"`
	}

	err := l.db.WithContext(ctx).
		Select("name, value, is_delete, update_time").
		Table("params_config").
		Where("update_time > ?", since).
		Find(&records).Error

	if err != nil {
		return LoadResult{}, err
	}

	data := make(map[string]string)
	maxUpdateTime := since // 默认保持上次的时间锚点，不回退

	if len(records) > 0 {
		logger.Info(ctx, "load since config", logit.IntField("nums", len(records)))
	}

	for _, r := range records {
		if r.IsDelete == 1 {
			data[r.Name] = "" // 标记删除
		} else {
			data[r.Name] = r.Value
		}
		if r.UpdateTime.After(maxUpdateTime) {
			maxUpdateTime = r.UpdateTime
		}
	}

	logger.Debug(ctx, "load since config detail", logit.IntField("nums", len(data)),
		logit.StringField("nextSince", maxUpdateTime.Format(time.RFC3339)), logit.JSONAbsField("data", data))

	return LoadResult{
		Data:      data,
		NextSince: maxUpdateTime,
	}, nil
}
