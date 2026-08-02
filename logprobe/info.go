package logprobe

import "encoding/json"

// ==========================================================================
// SourceInfo — 数据源自描述
// ==========================================================================

// SourceInfo 是 Info() 返回的自描述元数据。
//
// AI 在开始排查前调用 Info()，建立对数据源的认知：
//   - 有哪些字段可以查询和过滤？
//   - 数据覆盖的时间范围多大？（避免查未来或过久的数据）
//   - 大约有多少条日志？（判断宽搜是否会返回过多结果）
//   - 哪个字段是时间字段？哪个是消息主体？
//
// 这些信息让 AI 可以像人类工程师一样"了解数据源结构"后再查问题，
// 而不是盲目搜索碰运气。
type SourceInfo struct {
	// Name 人类可读的数据源名称，如 "prod-app-logs"。
	Name string `json:"name"`

	// Backend 底层存储引擎："elasticsearch"、"clickhouse"、"file"。
	// AI 可以据此调整查询策略（如 ES 支持 query_string 高级语法，
	// 而 CK 只能用 LIKE 模糊匹配）。
	Backend string `json:"backend"`

	// Fields 可查询和展示的字段列表。
	// 每个 FieldInfo 包含名称、类型、是否可检索、语义角色等。
	Fields []FieldInfo `json:"fields"`

	// TimeRange 数据的大致时间跨度。
	// 零值 TimeRange 表示无法确定（如远程文件未解析时间）。
	// AI 应据此设置搜索的时间范围，避免查不存在的时段。
	TimeRange TimeRange `json:"time_range"`

	// TimeField 主时间戳字段名（如 "@timestamp"、"ts"）。
	// 用于排序和 Stream 的游标定位。
	TimeField string `json:"time_field"`

	// MessageField 主日志消息字段名（如 "message"、"body"、"log"）。
	// 用于关键词检索的默认目标字段。
	MessageField string `json:"message_field"`

	// Estimated 预估日志条目总数。0 表示未知。
	// AI 用此字段判断宽搜的风险：
	//   - Estimated > 1 亿 + Query="ERROR" → 几乎肯定 IsExhaustive=false
	//   - Estimated < 1 万 → 可以放心宽搜
	Estimated int64 `json:"estimated_docs"`

	// Description 可选的人工描述。如 "生产环境应用日志，按天轮转"。
	Description string `json:"description,omitempty"`
}

// ==========================================================================
// FieldInfo — 字段元数据
// ==========================================================================

// FieldInfo 描述数据源中的一个字段。
//
// Roles 字段是本设计的核心：通过语义角色标注，Registry 可以自动发现
// 哪些数据源包含 trace_id 字段（用于跨数据源追踪），哪些字段适合做
// 时间排序、级别过滤等。
type FieldInfo struct {
	// Name 字段名 / 列名。
	Name string `json:"name"`

	// Type 数据类型："datetime"、"string"、"number"、"boolean"、"ip"。
	Type string `json:"type"`

	// Searchable 是否支持全文检索。
	// AI 对此类字段做关键词搜索才有效，对不可检索字段（如纯数字 ID）
	// 做全文搜索可能报错或无结果。
	Searchable bool `json:"searchable"`

	// Roles 语义角色标签。支持的值：
	//   - "timestamp": 时间字段，用于排序和游标定位
	//   - "trace_id": 分布式追踪 ID，Registry.Trace 据此自动发现数据源
	//   - "level": 日志级别（ERROR/WARN/INFO/DEBUG），适合 Stats 聚合
	//   - "message": 日志消息主体，默认关键词查询目标
	//   - "host": 主机标识
	//   - "service": 服务名
	// 一个字段可以有多个角色（如既是 timestamp 又是排序字段）。
	Roles []string `json:"roles,omitempty"`

	// Example 示例值。帮助 AI 理解字段的内容格式。
	// 如 "2026-06-22T10:30:00Z" 或 "192.168.1.1"。
	Example string `json:"example,omitempty"`

	// DistinctN 近似基数（不同值的数量）。0 表示未知。
	// AI 用此判断字段是否适合做 Stats 聚合：
	//   - DistinctN < 100 → 非常适合（如 level 通常只有几种值）
	//   - DistinctN ≈ 数据总量 → 不适合（如 trace_id 每个请求都不同）
	DistinctN int `json:"distinct_n,omitempty"`
}

// HasRole 判断字段是否携带指定角色标签。
// Registry.Trace 用此方法自动发现 trace_id 字段。
func (fi FieldInfo) HasRole(role string) bool {
	for _, r := range fi.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// ==========================================================================
// QueryError — 可指导 AI 修正的查询错误
// ==========================================================================

// QueryError 是可操作的查询错误。
//
// 与普通 error 只返回一段文字不同，QueryError 携带了 Hint（修正建议）
// 和 Example（修正后的请求 JSON），AI 读到后可以自动修正参数并重试，
// 不需要人工介入，也不需要靠"理解错误消息"来猜测怎么修正。
//
// JSON 示例：
//
//	{
//	    "message": "查询匹配了超过 120 万条日志（范围太宽）",
//	    "retryable": true,
//	    "hint": "添加时间范围或缩小关键词。可用的过滤维度: host, service, level",
//	    "example": "{\"query\":\"ERROR\",\"time_range\":{\"from\":\"2026-06-22T00:00:00Z\"}}"
//	}
type QueryError struct {
	// Message 人类可读的错误描述。
	Message string `json:"message"`

	// Retryable 表示 AI 是否可以修正参数后重试。
	// false 通常表示后端不可达、认证失败等不可恢复错误。
	Retryable bool `json:"retryable"`

	// Hint 修正建议。如"添加时间范围过滤"、"specified field is not searchable"。
	// AI 应据此确定修正方向。
	Hint string `json:"hint,omitempty"`

	// Example 修正后的请求 JSON 示例。直接可用的参考模板。
	Example json.RawMessage `json:"example,omitempty"`
}

// Error 实现 error 接口，返回人类可读的错误描述。
func (e *QueryError) Error() string { return e.Message }
