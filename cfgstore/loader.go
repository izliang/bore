package cfgstore

import (
	"context"
	"time"

	"github.com/izliang/bore/logit"
)

// LoadResult 配置加载结果
type LoadResult struct {
	Data      map[string]string // 配置数据，value为空表示删除该key
	NextSince time.Time         // 下次增量刷新的时间锚点
}

// Loader 配置加载器接口
// 可以是数据库、文件、远程服务等任意实现
type Loader interface {
	// LoadAll 全量加载所有配置（启动时调用）
	LoadAll(ctx context.Context, logger logit.Logger) (LoadResult, error)

	// LoadSince 增量加载 since 之后变更的配置（定时刷新调用）
	LoadSince(ctx context.Context, logger logit.Logger, since time.Time) (LoadResult, error)
}
