// Package logprobe（日志探针）是一个面向 AI Agent 的日志排查工具包。
//
// 它抽象了三种日志后端 — Elasticsearch (ELK)、ClickHouse、远程文件（本地或 SSH）—
// 统一为 LogSource 接口。每个方法都是一个单一意图的原语，AI 自行组合
// 成多步骤的排查工作流，无需了解后端差异。
//
// # 快速开始
//
//	// 创建 Registry，注册各个数据源
//	reg := logprobe.NewRegistry()
//	reg.Register(esadapter.NewSource(esConn, esadapter.Config{...}))
//	reg.Register(ckadapter.NewSource(ckConn, ckadapter.Config{...}))
//
//	// AI 第一步：发现数据源有哪些字段、什么时间范围
//	info, _ := reg.Get("prod-app").Info(ctx)
//
//	// 典型工作流：宽搜 → 统计分布 → 加过滤精准搜
//	r, _ := reg.Get("prod-app").Search(ctx, logprobe.SearchRequest{Query: "ERROR"})
//	if !r.IsExhaustive {
//	    // 结果被截断，AI 按 host 维度看分布
//	    stats, _ := reg.Get("prod-app").Stats(ctx, logprobe.StatsRequest{Field: "host"})
//	    // AI 自主决定：按 top host 加过滤条件重新搜索
//	}
//
//	// 流式追踪，支持断点续传
//	cur, _ := reg.Get("prod-app").Stream(ctx, logprobe.StreamRequest{
//	    SearchRequest: logprobe.SearchRequest{Query: "ERROR"},
//	    Follow:        true,
//	})
//	for batch, err := cur.Next(ctx); len(batch) > 0 || err == nil; batch, err = cur.Next(ctx) {
//	    for _, entry := range batch {
//	        // 处理每一条日志
//	    }
//	}
//	// 保存游标，进程重启后恢复
//	saveCheckpoint(cur.Cursor())
//
// 完整架构、AI 工作流和适配器设计见 DESIGN.md。
package logprobe

import (
	"time"

	"github.com/izliang/bore/appctx"
)

// ==========================================================================
// Format — 序列化格式
// ==========================================================================

// Format 控制 JSON 序列化的详细程度。
// AI 宿主通常使用 FormatCompact 以节省 token 消耗。
type Format int

const (
	// FormatFull 使用完整字段名，适合调试和人工阅读。
	// 例如：{"timestamp":"...","id":"...","fields":{"message":"..."}}
	FormatFull Format = iota

	// FormatCompact 使用缩写键名以节省 token。
	// 例如：{"ts":"...","id":"...","f":{"message":"..."}}
	// 与 FormatFull 相比，每条日志可节省约 30-40% 的 token。
	FormatCompact
)

// ==========================================================================
// LogSource — 核心抽象接口
// ==========================================================================

// LogSource 是所有日志后端的统一接口。
//
// 它暴露五个单一意图的原语（Info、Search、Stats、Context、Stream），
// 外加一个批量化字段展开操作（Expand）。AI Agent 将这些原语组合成
// 多步骤排查工作流，无需了解 ES DSL、SQL 或 grep 的语法差异。
//
// 每个适配器通过将通用请求转译成后端原生查询来实现此接口。
//
// 所有方法的第一个参数为 appctx.AppContext，它集成 context.Context 和日志能力：
//   - 可传递到任何接受 context.Context 的函数（AppContext 实现了该接口）
//   - 适配器内部可通过 atx.Logger().Debug(...) 进行调试日志输出
//   - 调用方可通过 atx.With(lctx.PushPair(atx, fields)) 注入请求级日志字段
//
// 接口设计原则：
//   - 只抽象三个后端真正共有的能力（时间排序 + 关键词过滤 + 流式输出），
//     不做最小公分母式的过度抽象。
//   - 每个方法做一件事，AI 自行规划调用顺序。
//   - 结果始终携带元数据（IsExhaustive、Cardinality），AI 据此决策而非猜测。
type LogSource interface {
	// ---- 发现 ----

	// ID 返回此数据源的唯一标识（例如 "prod-app-logs"）。
	// 用于游标断点恢复和跨数据源追踪。
	ID() string

	// Info 返回数据源的自描述元数据：可用字段列表、数据时间范围、
	// 预估条目数量等。AI 第一步调用此方法以建立对数据源的认知。
	// 没有这个调用，AI 不知道有哪些字段可以查询和过滤。
	Info(atx appctx.AppContext) (*SourceInfo, error)

	// ---- 查询 ----

	// Search 执行关键词检索，返回匹配的日志条目以及完整性元数据。
	//
	// 返回的 SearchResult 包含：
	//   - Entries: 匹配的日志条目列表
	//   - TotalHits: 后端匹配的总条数
	//   - IsExhaustive: false 表示返回的条目只是全部匹配的子集，
	//     AI 应该通过 Stats 找出合适的过滤维度来缩小范围
	//
	// Query 语义因后端而异：
	//   - ES: 对 message 字段做 query_string 检索
	//   - CK: WHERE message LIKE '%query%'（如有全文索引则优先使用）
	//   - 远程文件: grep -i 逐行匹配
	Search(atx appctx.AppContext, req SearchRequest) (*SearchResult, error)

	// Stats 对单个字段做 Top N 分布统计。
	//
	// AI 在宽搜后发现结果太多（IsExhaustive=false）时调用此方法，
	// 找出"哪个 host 的错误最多"、"哪个 service 占比最大"等信息，
	// 然后自主决定按 top 值加过滤条件精准搜索。
	//
	// 例如：先搜 "ERROR" 得到 12 万条 → Stats(field="host") →
	// 发现 80% 集中在 "db-prod-01" → AI 重新搜索
	// Search(query="ERROR", host="db-prod-01")。
	Stats(atx appctx.AppContext, req StatsRequest) (*StatsResult, error)

	// Context 获取指定日志条目的上下文（前后 N 条）。
	//
	// Scope 决定"上下文"的语义：
	//   - "time":  按时间相邻，前后各取 N 条。用于理解错误发生前后的日志流。
	//   - "trace": 同一条 trace/request 的所有日志，按时间排序。用于追踪请求链路。
	//   - "host":  同一主机的日志，按时间排序。用于排查单机问题。
	//
	// 返回值是 SearchResult，AI 可以对其中的长字段继续调用 Expand。
	Context(atx appctx.AppContext, req ContextRequest) (*SearchResult, error)

	// ---- 流式 ----

	// Stream 创建一个有状态的游标迭代器。
	//
	// 当 Follow=false 时：只取当前已有数据，取完后迭代器返回 ErrStreamDone 结束。
	// 当 Follow=true 时：取完已有数据后持续轮询新数据，直到 Close 或 atx 取消。
	//
	// 返回的 LogCursor 采用 Next() + Cursor() 模式，与 ck.CursorStream 完全对齐。
	// AI 的调用节奏是"调一次 → 拿一批 → 思考 → 再调一次"，
	// 游标迭代器天然匹配这种模式，无需开 goroutine 或管理 channel 生命周期。
	Stream(atx appctx.AppContext, req StreamRequest) (*LogCursor, error)

	// Cursor 返回当前流式消费位置，JSON 可序列化。
	// AI 可将其存入 Redis 或文件系统，进程重启后恢复。
	Cursor() Cursor

	// SetCursor 恢复之前保存的游标位置。
	// 传入零值 Cursor（Backend == ""）重置到起点。
	SetCursor(Cursor)

	// ---- 按需展开 ----

	// Expand 批量获取已截断字段的完整内容。
	//
	// 接受多个 ExpandTarget，一次网络往返展开多条日志的多个字段，
	// 避免 AI 逐条调用导致的 N 次往返延迟。
	//
	// 部分条目展开失败不影响其他条目，失败信息记录在 ExpandResult.Error 中。
	//
	// 使用场景：AI 先以 FieldMaxLen=200 搜索扫描大量日志，
	// 发现关键条目后，调用 Expand 获取完整堆栈信息。
	Expand(atx appctx.AppContext, targets []ExpandTarget) ([]ExpandResult, error)
}

// ==========================================================================
// Registry — 跨数据源协调
// ==========================================================================

// Registry 管理多个 LogSource，提供跨数据源操作（如分布式链路追踪）。
//
// 一个典型的线上排查场景涉及多个数据源：
//   - nginx 访问日志（ES 索引）
//   - 应用日志（CK 表）
//   - 慢查询日志（远程文件）
//
// Registry 统一管理这些数据源，AI 通过它获取任一数据源进行查询，
// 或通过 Trace 方法跨数据源追踪一条请求的完整链路。
type Registry struct {
	sources map[string]LogSource
}

// NewRegistry 创建一个空的 Registry。
func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]LogSource)}
}

// Register 注册一个 LogSource。重复注册相同 ID 会覆盖旧值。
func (r *Registry) Register(src LogSource) {
	r.sources[src.ID()] = src
}

// Get 按 ID 获取已注册的数据源。返回 nil, false 表示未找到。
func (r *Registry) Get(id string) (LogSource, bool) {
	src, ok := r.sources[id]
	return src, ok
}

// Trace 跨数据源分布式链路追踪。
//
// 按 trace_id 在所有已注册数据源中查找关联日志。
//
// 当 req.Sources 为空时：自动发现所有包含 trace_id 角色的数据源并发查询。
// 当 req.Sources 非空时：只在指定数据源中查询。
//
// 返回的 TraceResult 中 Spans 按 Timestamp 升序排列，便于 AI 按时间线
// 分析请求的完整路径和瓶颈所在。
//
// 当前为接口占位，具体实现在适配器层完成。
func (r *Registry) Trace(atx appctx.AppContext, req TraceRequest) (*TraceResult, error) {
	// 适配器层实现，此处仅定义类型契约供调用方依赖。
	_ = atx
	_ = req
	return nil, nil
}

// ==========================================================================
// TimeRange — 时间范围
// ==========================================================================

// TimeRange 表示一个左闭右开的时间区间 [From, To)。
// 零值表示不限制（即从最早到最晚，或从指定时间到无穷）。
type TimeRange struct {
	// From 起始时间（包含）。零值 = 不限起始。
	From time.Time `json:"from,omitempty"`

	// To 结束时间（不包含）。零值 = 不限结束。
	To time.Time `json:"to,omitempty"`
}

// IsZero 判断时间范围是否完全未设置（两端均为零值）。
func (tr TimeRange) IsZero() bool { return tr.From.IsZero() && tr.To.IsZero() }
