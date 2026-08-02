// Package escommon 提供 ES 查询 DSL 构建和响应解析的共享函数。
//
// 被 elasticsearch 和 kibanaproxy 两个适配器共同引用。
// 本包是纯函数层，无外部依赖（只依赖 logprobe 类型定义）。
// 两个适配器在本包基础上只替换传输层（HTTP 客户端 + 认证方式）。
package escommon

import (
	"fmt"
	"time"

	"github.com/izliang/bore/logprobe"
)

// ==========================================================================
// 查询体构建
// ==========================================================================

// BuildSearchBody 将 logprobe.SearchRequest 翻译为 ES _search 请求体。
//
// 翻译规则：
//   - Query → multi_match（跨字段短语匹配，type=phrase）
//   - Query 为空或 "*" → match_all
//   - TimeRange → range filter on @timestamp
//   - Fields → _source 过滤（只返回指定字段）
//   - Limit → size
//
// 返回值可直接 json.Marshal 后作为 _search body 使用。
// 时间字段名默认为 "@timestamp"，后续可通过 options 扩展。
func BuildSearchBody(req logprobe.SearchRequest, index string, opts ...SearchOption) map[string]any {
	o := applySearchOptions(opts)
	filters := make([]any, 0, 3)

	// 关键词 → multi_match
	if req.Query != "" && req.Query != "*" {
		qs := map[string]any{
			"query":   req.Query,
			"type":    "phrase",
			"lenient": true,
		}
		// 指定字段时限定搜索范围，否则 multi_match 跨所有字段搜索
		if len(o.searchFields) > 0 {
			qs["fields"] = o.searchFields
		}
		filters = append(filters, map[string]any{"multi_match": qs})
	}

	// 时间范围 → range filter
	if !req.TimeRange.IsZero() {
		r := map[string]any{}
		if !req.TimeRange.From.IsZero() {
			r["gte"] = req.TimeRange.From.Format(time.RFC3339Nano)
		}
		if !req.TimeRange.To.IsZero() {
			r["lte"] = req.TimeRange.To.Format(time.RFC3339Nano)
		}
		if len(r) > 0 {
			filters = append(filters, map[string]any{
				"range": map[string]any{o.timeField: r},
			})
		}
	}

	body := map[string]any{}

	// 构建 query 部分
	if len(filters) > 0 {
		body["query"] = map[string]any{
			"bool": map[string]any{"filter": filters},
		}
	} else {
		body["query"] = map[string]any{"match_all": map[string]any{}}
	}

	// 限制返回条数
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 10000 {
		limit = 10000
	}
	body["size"] = limit

	// _source 字段过滤
	if len(req.Fields) > 0 {
		// ES 端截断：FieldMaxLen > 0 时，非白名单字段使用 script_fields 在 ES 侧截断
		// 避免下载完整长字段，网络传输和反序列化开销降低 80%+
		if req.FieldMaxLen > 0 {
			expandSet := toSet(req.ExpandFields)
			// 时间字段始终走 _source（短字段，且 parseHit 需要从中提取时间戳）
			expandSet[o.timeField] = true
			sourceFields, sf := splitFieldsForTruncation(req.Fields, expandSet, req.FieldMaxLen)
			if len(sourceFields) > 0 {
				body["_source"] = sourceFields
			} else {
				// 全部字段走 script_fields 时用 false，ES 不接受 null
				body["_source"] = false
			}
			// 合并用户自定义 script_fields
			for k, v := range o.scriptFields {
				sf[k] = v
			}
			if len(sf) > 0 {
				body["script_fields"] = sf
			}
		} else {
			body["_source"] = req.Fields
		}
	}

	// 跟踪总数（用于 IsExhaustive 判断）
	track := true
	body["track_total_hits"] = &track

	// 无 _source 过滤时的 script_fields（用户自定义）
	if len(req.Fields) == 0 && len(o.scriptFields) > 0 {
		body["script_fields"] = o.scriptFields
	}

	return body
}

// BuildStatsBody 将 logprobe.StatsRequest 翻译为带 terms aggregation 的 _search 请求体。
//
// 过滤条件（Filter）复用 BuildSearchBody 的翻译逻辑。
// 聚合层用 terms aggregation，size=0 表示不返回 hits 只返回 aggregation。
func BuildStatsBody(req logprobe.StatsRequest, index string, opts ...SearchOption) map[string]any {
	o := applySearchOptions(opts)
	body := map[string]any{
		"size": 0,
	}

	// 构建过滤条件
	filters := make([]any, 0, 3)
	if req.Filter != nil {
		if req.Filter.Query != "" && req.Filter.Query != "*" {
			qs := map[string]any{
				"query":   req.Filter.Query,
				"type":    "phrase",
				"lenient": true,
			}
			if len(o.searchFields) > 0 {
				qs["fields"] = o.searchFields
			}
			filters = append(filters, map[string]any{"multi_match": qs})
		}
		if !req.Filter.TimeRange.IsZero() {
			r := map[string]any{}
			if !req.Filter.TimeRange.From.IsZero() {
				r["gte"] = req.Filter.TimeRange.From.Format(time.RFC3339Nano)
			}
			if !req.Filter.TimeRange.To.IsZero() {
				r["lte"] = req.Filter.TimeRange.To.Format(time.RFC3339Nano)
			}
			if len(r) > 0 {
				filters = append(filters, map[string]any{
					"range": map[string]any{o.timeField: r},
				})
			}
		}
	}

	if len(filters) > 0 {
		body["query"] = map[string]any{
			"bool": map[string]any{"filter": filters},
		}
	} else {
		body["query"] = map[string]any{"match_all": map[string]any{}}
	}

	// terms aggregation
	topN := req.TopN
	if topN <= 0 {
		topN = 20
	}
	body["aggs"] = map[string]any{
		"by_field": map[string]any{
			"terms": map[string]any{
				"field": req.Field,
				"size":  topN,
			},
		},
	}

	return body
}

// BuildContextBody 根据 ContextRequest 的 scope 构建上下文查询。
//
// Scope 翻译：
//   - "time": 以参考条目的时间戳为中心，range filter ±前后各取 N 条，按 @timestamp 排序。
//   - "trace": term filter on ScopeField（如 trace_id），按 @timestamp 升序。
//   - "host":  term filter on ScopeField（如 host.name），按 @timestamp 升序。
//
// refEntry 是 ContextRequest.EntryID 对应的 LogEntry，调用方需先通过 GET /_doc/{id} 获取。
func BuildContextBody(req logprobe.ContextRequest, refEntry logprobe.LogEntry, index string, opts ...SearchOption) map[string]any {
	o := applySearchOptions(opts)
	if req.Before <= 0 {
		req.Before = 20
	}
	if req.After <= 0 {
		req.After = 20
	}

	body := map[string]any{
		"size": req.Before + req.After + 1,
		"sort": []any{map[string]any{o.timeField: "asc"}},
	}

	switch req.Scope {
	case "trace":
		body["query"] = map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{req.ScopeField: refEntry.Fields[req.ScopeField].Text}},
				},
			},
		}
	case "host":
		body["query"] = map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"term": map[string]any{req.ScopeField: refEntry.Fields[req.ScopeField].Text}},
				},
			},
		}
		// 如有时间范围也加上
		if !refEntry.Timestamp.IsZero() {
			body["query"].(map[string]any)["bool"].(map[string]any)["filter"] = append(
				body["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any),
				map[string]any{"range": map[string]any{o.timeField: map[string]any{
					"gte": refEntry.Timestamp.Add(-1 * time.Hour).Format(time.RFC3339Nano),
					"lte": refEntry.Timestamp.Add(1 * time.Hour).Format(time.RFC3339Nano),
				}}},
			)
		}
	default: // "time"
		// 以参考条目的时间戳为中心，向前向后各取 Before+After 条
		// 简化实现：按时间排序 + from/size 做分页
		body["query"] = map[string]any{
			"bool": map[string]any{
				"filter": []any{
					map[string]any{"range": map[string]any{o.timeField: map[string]any{
						"gte": refEntry.Timestamp.Add(-5 * time.Minute).Format(time.RFC3339Nano),
						"lte": refEntry.Timestamp.Add(5 * time.Minute).Format(time.RFC3339Nano),
					}}},
				},
			},
		}
	}

	return body
}

// ==========================================================================
// SearchOption — 查询构建的可选参数
// ==========================================================================

// SearchOption 微调查询构建行为。
type SearchOption func(*searchOptions)

type searchOptions struct {
	timeField    string
	searchFields []string
	scriptFields map[string]any
}

func applySearchOptions(opts []SearchOption) *searchOptions {
	o := &searchOptions{
		timeField:    "@timestamp",
		searchFields: nil, // nil = 搜索所有字段
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithTimeField 指定时间字段名（默认 "@timestamp"）。
func WithTimeField(field string) SearchOption {
	return func(o *searchOptions) { o.timeField = field }
}

// WithSearchFields 指定 multi_match 搜索的目标字段（默认搜索所有字段）。
func WithSearchFields(fields ...string) SearchOption {
	return func(o *searchOptions) { o.searchFields = fields }
}

// WithScriptFields 注入原始 ES script_fields（高级用户自定义脚本计算）。
// 注意：script_fields 的值不可通过 logprobe.Expand 展开完整内容，
// 因为它们在 ES 侧计算完成时即为最终值。
func WithScriptFields(sf map[string]any) SearchOption {
	return func(o *searchOptions) { o.scriptFields = sf }
}

// splitFieldsForTruncation 将 Fields 拆分为 _source 字段（短字段/白名单）和 script_fields（ES 端截断）。
//
// 规则：
//   - expandSet 中的字段（ExpandFields 白名单）→ _source（不截断）
//   - 其余字段 → script_fields（ES Painless substring，只传输截断后的内容）
//
// 返回的 script_fields 中每个字段的值是一个 Painless map：{text, len, truncated}。
func splitFieldsForTruncation(fields []string, expandSet map[string]bool, maxLen int) (sourceFields []string, scriptFields map[string]any) {
	scriptFields = make(map[string]any)
	for _, field := range fields {
		if expandSet[field] {
			sourceFields = append(sourceFields, field)
			continue
		}
		// Painless 脚本：在 ES 侧 substring，返回结构化对象
		source := fmt.Sprintf(
			`def v = params._source['%s'];`+
				`if (v == null) return ['text': '', 'len': 0, 'truncated': false];`+
				`def s = v.toString();`+
				`return [`+
				`  'text': s.length() > %d ? s.substring(0, %d) : s,`+
				`  'len': s.length(),`+
				`  'truncated': s.length() > %d`+
				`];`, field, maxLen, maxLen, maxLen)
		scriptFields[field] = map[string]any{
			"script": map[string]any{
				"source": source,
				"lang":   "painless",
			},
		}
	}
	return
}

// ==========================================================================
// 错误定义
// ==========================================================================

// ErrUnsupportedScope 表示不受支持的 Context scope。
var ErrUnsupportedScope = fmt.Errorf("escommon: 不支持的 scope，仅支持 time/trace/host")
