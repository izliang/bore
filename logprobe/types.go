package logprobe

import (
	"context"
	"encoding/json"
	"time"
)

// ==========================================================================
// SearchRequest — 搜索请求
// ==========================================================================

// SearchRequest 定义一次关键词搜索。
//
// 各后端的 Query 转译方式：
//   - ES: 对 message 字段（或所有 searchable 字段）做 query_string 查询
//   - CK: WHERE message LIKE '%query%'（如配置了全文索引则优先使用）
//   - 远程文件: grep -i 逐行匹配
//
// 使用建议（面向 AI）：
//   - 首次搜索用较宽泛的关键词 + 最近 1 小时时间范围
//   - 如果 IsExhaustive=false，调用 Stats 找出合适的过滤维度
//   - 长字段（堆栈、请求体）初始用 FieldMaxLen=200 扫描，确认关键条目后再 Expand
type SearchRequest struct {
	// Query 关键词或查询字符串。必填。
	// 传 "*" 或空字符串表示匹配全部（等效于 match_all）。
	// 各后端按自身语法解释此字段。
	Query string `json:"query"`

	// Fields 指定要返回的字段列表。空切片 = 返回所有字段。
	// 合理使用可显著减少 token 消耗。
	// 例如：[]string{"@timestamp", "level", "message"}。
	Fields []string `json:"fields,omitempty"`

	// TimeRange 时间范围过滤。零值 = 不限时间。
	// 强烈建议 AI 每次搜索都带上时间范围，避免扫描全量数据。
	TimeRange TimeRange `json:"time_range,omitempty"`

	// Limit 返回条数上限。0 = 使用后端默认值。
	// ES 默认 10，CK 默认 500，远程文件默认 50。
	// 设置过大会浪费 token，建议 20-100。
	Limit int `json:"limit,omitempty"`

	// FieldMaxLen 每个字段的最大返回字符数。
	// 超过此长度的字段会被截断，FieldValue.Truncated 设为 true。
	// 默认 500。设为 0 表示不截断（慎用——堆栈信息可能撑爆 token 预算）。
	// AI 建议：第一轮扫描设 200，确认关键条目后通过 Expand 获取完整内容。
	FieldMaxLen int `json:"field_max_len,omitempty"`

	// ExpandFields 指定不截断的字段白名单。
	// 这些字段始终返回完整值，不受 FieldMaxLen 限制。
	// 例如：[]string{"error", "message"} 确保错误消息始终完整。
	ExpandFields []string `json:"expand_fields,omitempty"`

	// Format 控制响应序列化格式。
	// AI 宿主应使用 FormatCompact 以减少 token 消耗。
	Format Format `json:"format,omitempty"`
}

// ==========================================================================
// StreamRequest — 流式请求
// ==========================================================================

// StreamRequest 配置一个流式日志迭代器。
// 嵌入 SearchRequest 继承其所有过滤条件。
type StreamRequest struct {
	SearchRequest

	// Follow 控制已有数据消费完毕后是否持续追新。
	// true = 持续轮询新数据（实时监控模式）。false = 取完已有数据后结束。
	Follow bool `json:"follow"`
}

// ==========================================================================
// StatsRequest — 统计请求
// ==========================================================================

// StatsRequest 对单个字段做值分布统计，返回 Top N 桶。
//
// 典型 AI 工作流：
//   1. Search("ERROR") → IsExhaustive=false，12 万条太多
//   2. Stats(field="host", filter=上一步的查询) → 80% 集中在 db-prod-01
//   3. Stats(field="level") → Top: ERROR 6000, WARN 3800
//   4. AI 自主决策：Search("ERROR", host="db-prod-01", time=最近1小时) → 精准结果
type StatsRequest struct {
	// Filter 可选的过滤条件，统计前先缩小范围。
	// nil 表示对整个数据源做统计（通常用于了解数据源整体分布）。
	Filter *SearchRequest `json:"filter,omitempty"`

	// Field 要聚合的字段名（如 "level"、"host"、"service"）。
	// 应使用 logprobe_describe 返回的字段名。
	Field string `json:"field"`

	// TopN 返回的桶数量上限。默认 20。
	// 建议 AI 取 5-10 即可，过多会浪费 token。
	TopN int `json:"top_n,omitempty"`
}

// ==========================================================================
// ContextRequest — 上下文请求
// ==========================================================================

// ContextRequest 获取指定日志条目周边的上下文。
//
// 三种 Scope 语义：
//   - "time":  时间邻居。在参考条目的时间点前后各取 Before/After 条。
//              用于理解"错误发生前系统在做什么、错误发生后系统如何响应"。
//   - "trace": 同链路。查找同一 trace/request ID 的所有日志，按时间排序。
//              用于追踪一个请求的完整处理路径。
//   - "host":  同主机。查找同一主机上的日志，按时间排序。
//              用于判断是否是单机故障（同一时间段其他主机正常）。
type ContextRequest struct {
	// EntryID 参考日志条目 ID。来自之前的 Search 结果。必填。
	EntryID string `json:"entry_id"`

	// Before 向前取多少条。默认 20。
	Before int `json:"before"`

	// After 向后取多少条。默认 20。
	After int `json:"after"`

	// Scope 上下文维度："time"、"trace"、"host"。
	Scope string `json:"scope"`

	// ScopeField 用于 trace/host 范围的字段名。
	// 当 scope="trace" 时：如 "trace_id"。
	// 当 scope="host" 时：如 "host.name"。
	// 当 scope="time" 时：不需要此字段。
	ScopeField string `json:"scope_field,omitempty"`
}

// ==========================================================================
// TraceRequest — 跨数据源追踪请求
// ==========================================================================

// TraceRequest 执行跨数据源分布式链路追踪。
// 给定一个 trace_id，在所有（或指定）数据源中查找关联日志条目。
//
// 使用场景：一个线上请求穿过 nginx → app → DB，
// 三个数据源各存一段日志。AI 通过此方法串联完整链路。
type TraceRequest struct {
	// TraceID 分布式追踪标识。必填。
	TraceID string `json:"trace_id"`

	// Sources 限定查询的数据源列表。空 = 自动发现所有包含 trace_id 字段的数据源。
	Sources []string `json:"sources,omitempty"`

	// MaxSpans 跨所有数据源返回的 span 总数上限。0 = 不限制。
	MaxSpans int `json:"max_spans,omitempty"`
}

// ==========================================================================
// SearchResult — 搜索结果
// ==========================================================================

// SearchResult 封装匹配的日志条目及完整性元数据。
//
// 关键：AI 不应假设返回的条目就是全部匹配结果。
// 必须检查 IsExhaustive——如果为 false，说明后端实际匹配了更多条目，
// AI 需要加过滤条件（时间、host、level）精准搜索，或通过 Stats 了解分布。
type SearchResult struct {
	// Entries 匹配的日志条目列表。按时间排序（升序或降序取决于请求配置）。
	Entries []LogEntry `json:"entries"`

	// TotalHits 后端匹配的总条数。
	// 注意：当 IsExhaustive=false 时，此值可能为近似值或被截断上限。
	// 例如 ES 默认只精确追踪到 10,000 条，超出的返回 "10000+".
	TotalHits int64 `json:"total_hits"`

	// IsExhaustive 表示 TotalHits 是否精确。
	// false = 实际匹配数可能超过 TotalHits，AI 应使用 Stats 缩小范围。
	// true  = TotalHits 是精确值，AI 可放心使用。
	IsExhaustive bool `json:"is_exhaustive"`

	// ReturnedHits 本次实际返回的条目数（通常 ≤ Limit）。
	ReturnedHits int `json:"returned_hits"`

	// TookMs 后端查询耗时（毫秒）。可用于性能分析。
	TookMs int64 `json:"took_ms"`

	// Suggestions 当结果集过大时提供的聚合建议。
	// 每个建议提示一个适合做过滤的字段及其 Top 值分布。
	// nil 表示无建议。
	Suggestions []AggSuggestion `json:"suggestions,omitempty"`
}

// AggSuggestion 聚合建议：提示 AI 按哪个字段过滤可以快速缩小范围。
type AggSuggestion struct {
	// Field 建议的过滤字段名。
	Field string `json:"field"`

	// Top 该字段的 Top N 值及各自的数量。
	Top []BucketItem `json:"top"`
}

// BucketItem 统计桶中的一个条目：某个值出现了多少次。
type BucketItem struct {
	// Value 字段值。
	Value string `json:"value"`

	// Count 该值出现的次数。
	Count int64 `json:"count"`
}

// ==========================================================================
// StatsResult — 统计结果
// ==========================================================================

// StatsResult 是单个字段的值分布统计结果。
// Buckets 按 Count 降序排列，Cardinality 表示不同值的总数（近似）。
type StatsResult struct {
	// Field 被统计的字段名。
	Field string `json:"field"`

	// Cardinality 近似不同值总数。
	// 0 表示无法计算（后端不支持或字段基数过高）。
	Cardinality int `json:"cardinality"`

	// Buckets Top N 值分布，按 Count 降序排列。
	Buckets []BucketItem `json:"buckets"`
}

// ==========================================================================
// TraceResult — 链路追踪结果
// ==========================================================================

// TraceResult 包含跨数据源的分布式链路追踪结果。
type TraceResult struct {
	// TraceID 追踪标识。
	TraceID string `json:"trace_id"`

	// Spans 所有 span，按 Timestamp 升序排列。
	// 每个 span 携带 Source 字段标识其来源数据源。
	// AI 可以按时间轴分析请求在各阶段的耗时。
	Spans []TraceSpan `json:"spans"`
}

// TraceSpan 分布式链路中的一条日志记录，标注了来源数据源。
type TraceSpan struct {
	// Source 来自哪个数据源（LogSource.ID() 的值）。
	Source string `json:"source"`

	// EntryID 在此数据源中的日志条目 ID。
	EntryID string `json:"entry_id"`

	// Timestamp 日志时间。
	Timestamp time.Time `json:"timestamp"`

	// Fields 日志字段。长字段可能被截断，AI 可调用 Expand 获取完整内容。
	Fields map[string]FieldValue `json:"fields"`
}

// ==========================================================================
// LogEntry — 统一日志条目
// ==========================================================================

// LogEntry 是所有后端返回的统一日志记录。
//
// 对于远程文件（无结构化字段），Fields 中只有一个键 "message"，
// 其值为整行原始文本。
//
// Raw 字段保留了后端的原始响应（ES 的完整 hit、CK 的原始行映射），
// 高级调用方可以自行解析以使用后端特有功能（如 ES 的高亮、评分）。
type LogEntry struct {
	// Timestamp 日志事件时间。所有后端都必须能提供此字段。
	Timestamp time.Time `json:"timestamp"`

	// ID 在此后端中的唯一标识。
	// 用于 Context 的参考条目定位和 Expand 的按 ID 查找。
	ID string `json:"id"`

	// Fields 字段名 → 字段值（带截断元数据）的映射。
	// 对于远程文件，只有 "message" 一个键。
	Fields map[string]FieldValue `json:"fields"`

	// Raw 后端原始响应。为高级调用方保留，nil 表示未保留原始数据。
	// 可以二次解析以使用后端特有功能（如 ES 的高亮、评分）。
	Raw json.RawMessage `json:"raw,omitempty"`
}

// ==========================================================================
// FieldValue — 字段值（支持截断与懒加载）
// ==========================================================================

// FieldValue 封装一个日志字段的值，附带截断信息并按需懒加载完整内容。
//
// 两种构造方式：
//   - NewFieldValue(full, maxLen): 完整值已在内存中，截断零开销，Expand 直接返回。
//     适用于 CK（列存天然低开销）和远程文件（行即全部）。
//   - NewLazyFieldValue(prefix, fullLen, fn): 完整值通过 fn 惰性加载。
//     适用于 ES（大字段需要单独 GET doc 获取）。
//
// 截断策略：
//   - Text 存储字段文本。未截断时即为完整值，截断时仅为前缀。
//   - Truncated=true 时，AI 应展示 Text + "...（N 字符）" + 展开入口。
//   - Expand() 按需获取完整内容。截断状态下无 expander 时返回前缀（降级处理）。
type FieldValue struct {
	// Text 字段内容。截断时是前缀，未截断时是完整值。
	Text string `json:"text"`

	// Truncated 为 true 表示 Text 仅为完整值的前缀。
	Truncated bool `json:"truncated"`

	// FullLength 完整值的原始字节长度。仅在 Truncated=true 时有意义。
	// AI 可用于展示 "...（2048 字符）" 等提示。
	FullLength int `json:"full_length"`

	// expander 内部闭包，按需获取完整字段内容。
	// nil 表示完整内容已在 Text 中，或无法懒加载。
	expander func(ctx context.Context) (string, error)
}

// Expand 获取字段的完整内容。
//
// 行为：
//   - Truncated=false: 直接返回 Text（零开销）。
//   - Truncated=true 且有 expander: 调用 expander 懒加载（可能发网络请求）。
//   - Truncated=true 但无 expander: 降级返回 Text 前缀。
//
// 对于 CK 和远程文件，完整值已在内存中，Expand 始终零开销。
// 对于 ES，如果字段是通过 _source 过滤以缩略返回的，
// Expand 会发一次 GET /index/_doc/id?_source_includes=field 请求。
func (fv FieldValue) Expand(ctx context.Context) (string, error) {
	if !fv.Truncated || fv.expander == nil {
		return fv.Text, nil
	}
	return fv.expander(ctx)
}

// NewFieldValue 创建一个可能被截断的 FieldValue。
//
// 完整值已在参数 full 中，内部通过闭包持有，Expand 时直接返回，无网络开销。
// 适用于 CK（SQL 已返回完整行数据）和远程文件（行已全部读取）。
//
// 参数：
//   - full: 完整字段值。
//   - maxLen: 截断阈值。≤0 表示不截断；full 长度 ≤ maxLen 时也不截断。
func NewFieldValue(full string, maxLen int) FieldValue {
	if maxLen <= 0 || len(full) <= maxLen {
		return FieldValue{Text: full, FullLength: len(full)}
	}
	fv := FieldValue{
		Text:       full[:maxLen],
		Truncated:  true,
		FullLength: len(full),
	}
	fullCopy := full
	fv.expander = func(_ context.Context) (string, error) { return fullCopy, nil }
	return fv
}

// NewLazyFieldValue 创建一个惰性加载的 FieldValue。
//
// 完整内容仅在 Expand 被调用时通过 fn 获取（通常是一次网络请求）。
// 适用于需要按需加载完整大字段的场景，如 ES 的单独 GET doc 请求。
//
// 参数：
//   - prefix: 已展示的前缀文本（如搜索结果中返回的截断值）。
//   - fullLen: 完整内容的字节长度（用于展示"...（N 字符）"）。
//   - fn: 惰性加载函数，接收 ctx，返回完整值。
func NewLazyFieldValue(prefix string, fullLen int, fn func(ctx context.Context) (string, error)) FieldValue {
	return FieldValue{
		Text:       prefix,
		Truncated:  true,
		FullLength: fullLen,
		expander:   fn,
	}
}

// ==========================================================================
// ExpandTarget / ExpandResult — 字段展开的请求与响应
// ==========================================================================

// ExpandTarget 标识一条日志中的一个字段，用于 Expand 批量请求。
type ExpandTarget struct {
	// EntryID 日志条目 ID。来自之前的 Search 或 Stream 结果。
	EntryID string `json:"entry_id"`

	// Field 要展开的字段名（如 "stack_trace"、"message"、"request_body"）。
	Field string `json:"field"`
}

// ExpandResult 展开单个字段的结果。
// 单条展开失败通过 Error 字段传达，不影响同批次其他条目的展开。
// 调用方应检查每个 ExpandResult 的 Error 字段。
type ExpandResult struct {
	// EntryID 对应的日志条目 ID。
	EntryID string `json:"entry_id"`

	// Field 被展开的字段名。
	Field string `json:"field"`

	// Value 完整字段内容。仅在 Error 为空时有效。
	Value string `json:"value"`

	// Error 展开失败的错误信息。空字符串表示成功。
	// 部分失败不影响同批次其他 ExpandTarget。
	Error string `json:"error,omitempty"`
}
