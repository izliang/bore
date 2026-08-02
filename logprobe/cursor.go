package logprobe

import (
	"context"
	"encoding/json"
	"errors"

)

// ==========================================================================
// Cursor — 可持久化的流式位置
// ==========================================================================

// Cursor 是后端无关的流式消费位置，JSON 可序列化，适合存入 Redis、
// 文件系统等持久化存储，实现断点续传。
//
// 零值 Cursor（Backend == ""）表示"从头开始消费"。
//
// 各适配器的 Position 格式：
//
//	CK:
//	  {"backend":"ck","position":{"value":"2026-06-22 10:30:00","values":["2026-06-22 10:30:00",42]}}
//
//	ES:
//	  {"backend":"es","position":["2026-06-22T10:30:00Z","doc-123"]}
//
//	远程文件:
//	  {"backend":"file","position":{"path":"/var/log/app.log","offset":1048576}}
//
// Backend 字段用于恢复时路由到正确的数据源，Position 的格式由各适配器自定。
type Cursor struct {
	// Backend 标识创建此游标的适配器。
	// 恢复时根据此字段将游标路由到正确的 LogSource。
	Backend string `json:"backend"`

	// Position 适配器特定的游标数据，格式各适配器自定。
	// 调用方不应直接解析此字段，应将其整体传给 SetCursor。
	Position json.RawMessage `json:"position"`
}

// IsZero 判断游标是否从未推进（零值游标）。
// 返回 true 表示"从头开始"。
func (c Cursor) IsZero() bool { return c.Backend == "" }

// ==========================================================================
// LogCursor — 有状态流式迭代器
// ==========================================================================

// LogCursor 是 LogSource.Stream 返回的有状态迭代器。
//
// 采用 Next() + Cursor() 模式，与 ck.CursorStream 完全对齐。
// AI 的调用节奏是"调一次 Next → 拿到一批 → 分析思考 → 再调 Next"，
// 游标迭代器天然匹配这种模式：不需要开 goroutine，不需要管理 channel 生命周期。
//
// 使用示例：
//
//	cur, err := src.Stream(ctx, logprobe.StreamRequest{
//	    SearchRequest: logprobe.SearchRequest{Query: "ERROR"},
//	    Follow:        true,
//	})
//	defer cur.Close()
//
//	for {
//	    batch, err := cur.Next(ctx)
//	    if err != nil {
//	        if errors.Is(err, logprobe.ErrStreamDone) {
//	            break // Follow=false 模式下，所有已有数据消费完毕
//	        }
//	        return err // 流异常，停止
//	    }
//	    if len(batch) == 0 {
//	        // Follow=true 模式下暂无新数据，AI 可等待后重试
//	        // 或在此阻塞。
//	        continue
//	    }
//	    for _, entry := range batch {
//	        handle(entry)
//	    }
//	    // 消费完一批后立即保存游标，防止崩溃丢失进度
//	    saveCheckpoint(cur.Cursor())
//	}
type LogCursor struct {
	// Next 获取下一批日志条目。
	//
	// 返回语义：
	//   - 返回非空批次 + nil error: 正常数据，逐条处理后继续调用。
	//   - 返回空批次 + nil error: 暂无新数据（Follow=true 模式），稍后重试。
	//   - 返回空批次 + ErrStreamDone: 所有已有数据消费完毕（Follow=false 模式）。
	//   - 返回非 nil error（非哨兵错误）: 流异常，应停止消费。
	//
	// Next 是否阻塞取决于适配器实现。轮询模式（CK、ES）通常立即返回；
	// 远程文件模式可能阻塞等待新数据。
	Next func(ctx context.Context) ([]LogEntry, error)

	// Cursor 返回当前消费位置，JSON 可序列化。
	// AI 应在每批处理完后立即调用并保存，防止进程崩溃丢失进度。
	Cursor func() Cursor

	// Close 释放后端资源（ES 的 PIT、远程文件的句柄等）。
	// 使用 defer cur.Close() 确保资源释放。
	Close func() error
}

// ErrStreamDone 表示非跟随模式（Follow=false）下所有已有数据已消费完毕。
// Follow=true 的流永远不会返回此错误。
var ErrStreamDone = errors.New("logprobe: 流已结束")
