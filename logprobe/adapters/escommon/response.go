package escommon

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/izliang/bore/logprobe"
)

// ==========================================================================
// ES 响应结构（内部使用，简化版）
// ==========================================================================

type esSearchResponse struct {
	Hits esHits          `json:"hits"`
	Took int64           `json:"took"`
	Aggs json.RawMessage `json:"aggregations,omitempty"`
}

type esHits struct {
	Total esTotal `json:"total"`
	Hits  []esHit `json:"hits"`
}

type esTotal struct {
	Value    int64  `json:"value"`
	Relation string `json:"relation"` // "eq" = 精确, "gte" = 近似（达到上限）
}

type esHit struct {
	ID     string           `json:"_id"`
	Index  string           `json:"_index"`
	Source json.RawMessage  `json:"_source"`
	Fields map[string][]any `json:"fields,omitempty"` // script_fields 结果
	Sort   []any            `json:"sort,omitempty"`
}

// ==========================================================================
// ParseSearchResult — 解析 _search 响应
// ==========================================================================

// ParseSearchResult 解析 ES _search 响应为 logprobe.SearchResult。
//
// 自动处理：
//   - fieldMaxLen 截断（expandFields 中的字段不截断）
//   - script_fields 结果注入 Fields（优先级低于 _source 中的真实字段）
//   - IsExhaustive 根据 total.relation 判定（"eq"=true, "gte"=false）
//
// 参数：
//   - body: _search 响应的 JSON reader
//   - fieldMaxLen: 截断阈值（0 不截断）
//   - expandFields: 不截断的白名单字段
//   - timeField: 用于提取时间戳的字段名
func ParseSearchResult(body io.Reader, fieldMaxLen int, expandFields []string, timeField string) (*logprobe.SearchResult, error) {
	var esResp esSearchResponse
	if err := json.NewDecoder(body).Decode(&esResp); err != nil {
		return nil, fmt.Errorf("escommon: 解析 _search 响应失败: %w", err)
	}

	entries := make([]logprobe.LogEntry, 0, len(esResp.Hits.Hits))
	expandSet := toSet(expandFields)
	for _, hit := range esResp.Hits.Hits {
		entry := parseHit(hit, timeField)

		// 合并 script_fields 结果到 Fields
		mergeScriptFields(&entry, hit.Fields, fieldMaxLen, expandSet)

		entries = append(entries, entry)
	}

	// 统一截断
	for i := range entries {
		ApplyFieldMaxLenToEntry(&entries[i], fieldMaxLen, expandSet)
	}

	// 构建聚合建议（如果 total 很大）
	var suggestions []logprobe.AggSuggestion
	if esResp.Aggs != nil {
		suggestions = parseAggSuggestions(esResp.Aggs)
	}

	return &logprobe.SearchResult{
		Entries:      entries,
		TotalHits:    esResp.Hits.Total.Value,
		IsExhaustive: esResp.Hits.Total.Relation == "eq",
		ReturnedHits: len(entries),
		TookMs:       esResp.Took,
		Suggestions:  suggestions,
	}, nil
}

// parseHit 将单个 ES hit 解析为 LogEntry。
func parseHit(hit esHit, timeField string) logprobe.LogEntry {
	entry := logprobe.LogEntry{
		ID:     hit.ID,
		Fields: make(map[string]logprobe.FieldValue),
	}

	// 解析 _source 为 map
	if hit.Source != nil {
		var src map[string]any
		if err := json.Unmarshal(hit.Source, &src); err == nil {
			for k, v := range src {
				text := fmt.Sprint(v)
				entry.Fields[k] = logprobe.FieldValue{
					Text:       text,
					FullLength: len(text),
				}
				// 提取时间戳
				if k == timeField {
					if t, err := parseTimestamp(v); err == nil {
						entry.Timestamp = t
					}
				}
			}
		}
	}

	// 从 sort 值推断时间戳（_source 未包含 timeField 时）
	if entry.Timestamp.IsZero() && len(hit.Sort) > 0 {
		if ts, ok := hit.Sort[0].(float64); ok {
			entry.Timestamp = time.UnixMilli(int64(ts))
		}
	}

	return entry
}

// mergeScriptFields 将 script_fields 结果合并到 LogEntry 的 Fields 中。

// parseScriptFieldValue 解析 script_field 返回的单个值。
//
// ES 端截断的字段返回 Painless map：{text: "...", len: 636, truncated: true}
// 用户自定义 script_fields 返回普通字符串。
// 自动识别两种格式并构建 FieldValue。
func parseScriptFieldValue(val any) logprobe.FieldValue {
	if val == nil {
		return logprobe.FieldValue{}
	}

	// 尝试解析为结构化截断对象 {text, len, truncated}
	if m, ok := val.(map[string]any); ok {
		text, _ := m["text"].(string)
		length := 0
		if l, ok := m["len"].(float64); ok {
			length = int(l)
		}
		truncated, _ := m["truncated"].(bool)
		return logprobe.FieldValue{
			Text:       text,
			FullLength: length,
			Truncated:  truncated,
		}
	}

	// 普通值（用户自定义 script_field），转为字符串
	text := fmt.Sprint(val)
	return logprobe.FieldValue{
		Text:       text,
		FullLength: len(text),
	}
}

// script_fields 的值是数组 [val]，取第一个元素。
// 如果 _source 中已有同名字段则不覆盖。
func mergeScriptFields(entry *logprobe.LogEntry, fields map[string][]any, maxLen int, expandSet map[string]bool) {
	for name, vals := range fields {
		if _, exists := entry.Fields[name]; exists {
			continue // _source 已有，不覆盖
		}
		if len(vals) == 0 {
			continue
		}
		fv := parseScriptFieldValue(vals[0])
		if fv.Text == "" && fv.FullLength == 0 {
			continue // 空值跳过
		}
		entry.Fields[name] = fv
	}
}

// parseAggSuggestions 从 aggregation 结果构建 AggSuggestion 列表。
func parseAggSuggestions(aggs json.RawMessage) []logprobe.AggSuggestion {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(aggs, &raw); err != nil {
		return nil
	}
	var suggestions []logprobe.AggSuggestion
	for field, data := range raw {
		var bucket struct {
			Buckets []struct {
				Key      string `json:"key"`
				DocCount int64  `json:"doc_count"`
			} `json:"buckets"`
		}
		if json.Unmarshal(data, &bucket) == nil && len(bucket.Buckets) > 0 {
			agg := logprobe.AggSuggestion{Field: field}
			for _, b := range bucket.Buckets {
				agg.Top = append(agg.Top, logprobe.BucketItem{Value: b.Key, Count: b.DocCount})
			}
			suggestions = append(suggestions, agg)
		}
	}
	return suggestions
}

// ==========================================================================
// ParseStatsResult — 解析 terms aggregation 响应
// ==========================================================================

// ParseStatsResult 解析 terms aggregation 的 _search 响应为 logprobe.StatsResult。
func ParseStatsResult(body io.Reader, field string) (*logprobe.StatsResult, error) {
	var esResp struct {
		Aggregations struct {
			ByField struct {
				SumOtherDocCount int64 `json:"sum_other_doc_count"`
				Buckets          []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"by_field"`
		} `json:"aggregations"`
	}
	if err := json.NewDecoder(body).Decode(&esResp); err != nil {
		return nil, fmt.Errorf("escommon: 解析 aggregation 响应失败: %w", err)
	}

	buckets := esResp.Aggregations.ByField.Buckets
	result := &logprobe.StatsResult{
		Field:       field,
		Cardinality: len(buckets) + int(esResp.Aggregations.ByField.SumOtherDocCount),
		Buckets:     make([]logprobe.BucketItem, 0, len(buckets)),
	}
	for _, b := range buckets {
		result.Buckets = append(result.Buckets, logprobe.BucketItem{
			Value: b.Key,
			Count: b.DocCount,
		})
	}
	return result, nil
}

// ==========================================================================
// ParseDocResponse — 解析 GET /{index}/_doc/{id} 响应
// ==========================================================================

// ParseDocResponse 解析单文档 GET 响应为 LogEntry。
func ParseDocResponse(body io.Reader, timeField string) (*logprobe.LogEntry, error) {
	var resp struct {
		ID     string          `json:"_id"`
		Found  bool            `json:"found"`
		Source json.RawMessage `json:"_source"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, fmt.Errorf("escommon: 解析 get doc 响应失败: %w", err)
	}
	if !resp.Found {
		return nil, fmt.Errorf("escommon: 文档未找到")
	}
	entry := &logprobe.LogEntry{
		ID:     resp.ID,
		Fields: make(map[string]logprobe.FieldValue),
	}
	if resp.Source != nil {
		var src map[string]any
		if err := json.Unmarshal(resp.Source, &src); err == nil {
			for k, v := range src {
				text := fmt.Sprint(v)
				entry.Fields[k] = logprobe.FieldValue{
					Text:       text,
					FullLength: len(text),
				}
				if k == timeField {
					if t, err := parseTimestamp(v); err == nil {
						entry.Timestamp = t
					}
				}
			}
		}
	}
	return entry, nil
}

// ==========================================================================
// 辅助函数
// ==========================================================================

// parseTimestamp 尝试将多种格式的值解析为 time.Time。
func parseTimestamp(v any) (time.Time, error) {
	switch val := v.(type) {
	case string:
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000Z",
			"2006-01-02T15:04:05Z",
			"2006-01-02 15:04:05.000",
			"2006-01-02 15:04:05",
		}
		for _, f := range formats {
			if t, err := time.Parse(f, val); err == nil {
				return t, nil
			}
		}
		return time.Time{}, fmt.Errorf("无法解析时间: %s", val)
	case float64:
		// Unix 毫秒
		return time.UnixMilli(int64(val)), nil
	default:
		return time.Time{}, fmt.Errorf("不支持的时间类型: %T", v)
	}
}

// toSet 将 []string 转换为 set map。
func toSet(s []string) map[string]bool {
	m := make(map[string]bool, len(s))
	for _, item := range s {
		m[item] = true
	}
	return m
}

// ApplyFieldMaxLenToEntry 对单条 LogEntry 的所有字段应用截断。
func ApplyFieldMaxLenToEntry(entry *logprobe.LogEntry, maxLen int, expandFields map[string]bool) {
	if maxLen <= 0 {
		return
	}
	for name, fv := range entry.Fields {
		if expandFields[name] {
			continue
		}
		if fv.Truncated {
			continue
		}
		if len(fv.Text) > maxLen {
			entry.Fields[name] = logprobe.NewFieldValue(fv.Text, maxLen)
		}
	}
}
