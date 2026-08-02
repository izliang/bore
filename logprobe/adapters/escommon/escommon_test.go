package escommon

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/izliang/bore/logprobe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// BuildSearchBody 测试
// ==========================================================================

func TestBuildSearchBody_KeywordQuery(t *testing.T) {
	req := logprobe.SearchRequest{
		Query: "NullPointerException",
		Limit: 50,
	}
	body := BuildSearchBody(req, "logs-*")
	query := body["query"].(map[string]any)
	boolQuery := query["bool"].(map[string]any)
	filters := boolQuery["filter"].([]any)
	assert.Len(t, filters, 1)
	qs := filters[0].(map[string]any)["multi_match"].(map[string]any)
	assert.Equal(t, "NullPointerException", qs["query"])
	assert.Equal(t, "phrase", qs["type"])
	assert.Equal(t, 50, body["size"])
}

func TestBuildSearchBody_MatchAll(t *testing.T) {
	req := logprobe.SearchRequest{Query: "*"}
	body := BuildSearchBody(req, "logs-*")
	assert.Contains(t, body["query"].(map[string]any), "match_all")
}

func TestBuildSearchBody_EmptyQuery(t *testing.T) {
	req := logprobe.SearchRequest{}
	body := BuildSearchBody(req, "logs-*")
	assert.Contains(t, body["query"].(map[string]any), "match_all")
}

func TestBuildSearchBody_TimeRange(t *testing.T) {
	req := logprobe.SearchRequest{
		Query: "error",
		TimeRange: logprobe.TimeRange{
			From: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC),
			To:   time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC),
		},
	}
	body := BuildSearchBody(req, "logs-*")
	filters := body["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	assert.Len(t, filters, 2) // query_string + range
}

func TestBuildSearchBody_SourceFields(t *testing.T) {
	req := logprobe.SearchRequest{
		Query:  "error",
		Fields: []string{"@timestamp", "message"},
	}
	body := BuildSearchBody(req, "logs-*")
	source := body["_source"].([]string)
	assert.Equal(t, "@timestamp", source[0])
	assert.Equal(t, "message", source[1])
}

func TestBuildSearchBody_EmptyFieldsNotIncluded(t *testing.T) {
	req := logprobe.SearchRequest{Query: "error"}
	body := BuildSearchBody(req, "logs-*")
	assert.NotContains(t, body, "_source")
}

func TestBuildSearchBody_LimitCap(t *testing.T) {
	req := logprobe.SearchRequest{Query: "error", Limit: 20000}
	body := BuildSearchBody(req, "logs-*")
	assert.Equal(t, 10000, body["size"])
}

func TestBuildSearchBody_LimitDefault(t *testing.T) {
	req := logprobe.SearchRequest{Query: "error"}
	body := BuildSearchBody(req, "logs-*")
	assert.Equal(t, 100, body["size"])
}

func TestBuildSearchBody_ScriptFields(t *testing.T) {
	req := logprobe.SearchRequest{Query: "error"}
	sf := map[string]any{
		"msg_preview": map[string]any{"script": map[string]any{"source": "return 'test';"}},
	}
	body := BuildSearchBody(req, "logs-*", WithScriptFields(sf))
	assert.Contains(t, body, "script_fields")
}

func TestBuildSearchBody_WithTimeField(t *testing.T) {
	req := logprobe.SearchRequest{
		Query:     "error",
		TimeRange: logprobe.TimeRange{From: time.Now()},
	}
	body := BuildSearchBody(req, "logs-*", WithTimeField("ts"))
	filters := body["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	// 第二个 filter 应该是 range on ts
	rangeFilter := filters[1].(map[string]any)["range"].(map[string]any)
	assert.Contains(t, rangeFilter, "ts")
}

// ==========================================================================
// BuildStatsBody 测试
// ==========================================================================

func TestBuildStatsBody(t *testing.T) {
	req := logprobe.StatsRequest{
		Field: "level",
		TopN:  10,
	}
	body := BuildStatsBody(req, "logs-*")
	assert.Equal(t, 0, body["size"])
	aggs := body["aggs"].(map[string]any)
	terms := aggs["by_field"].(map[string]any)["terms"].(map[string]any)
	assert.Equal(t, "level", terms["field"])
	assert.Equal(t, 10, terms["size"])
}

func TestBuildStatsBody_WithFilter(t *testing.T) {
	req := logprobe.StatsRequest{
		Filter: &logprobe.SearchRequest{Query: "ERROR"},
		Field:  "host",
		TopN:   5,
	}
	body := BuildStatsBody(req, "logs-*")
	query := body["query"].(map[string]any)
	assert.Contains(t, query, "bool")
}

// ==========================================================================
// BuildContextBody 测试
// ==========================================================================

func TestBuildContextBody_TimeScope(t *testing.T) {
	ref := logprobe.LogEntry{
		Timestamp: time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC),
	}
	req := logprobe.ContextRequest{
		EntryID: "abc123",
		Before:  10,
		After:   10,
		Scope:   "time",
	}
	body := BuildContextBody(req, ref, "logs-*")
	query := body["query"].(map[string]any)
	assert.Contains(t, query, "bool")
}

func TestBuildContextBody_TraceScope(t *testing.T) {
	ref := logprobe.LogEntry{
		Fields: map[string]logprobe.FieldValue{
			"trace_id": {Text: "trace-xyz"},
		},
	}
	req := logprobe.ContextRequest{
		EntryID:    "abc123",
		Before:     10,
		After:      10,
		Scope:      "trace",
		ScopeField: "trace_id",
	}
	body := BuildContextBody(req, ref, "logs-*")
	filters := body["query"].(map[string]any)["bool"].(map[string]any)["filter"].([]any)
	term := filters[0].(map[string]any)["term"].(map[string]any)["trace_id"]
	assert.Equal(t, "trace-xyz", term)
}

// ==========================================================================
// ParseSearchResult 测试
// ==========================================================================

func TestParseSearchResult(t *testing.T) {
	esJSON := `{
		"took": 42,
		"hits": {
			"total": {"value": 3, "relation": "eq"},
			"hits": [
				{"_id":"1","_source":{"@timestamp":"2026-06-22T10:00:00Z","message":"error 1","level":"ERROR"}},
				{"_id":"2","_source":{"@timestamp":"2026-06-22T10:01:00Z","message":"error 2","level":"WARN"}},
				{"_id":"3","_source":{"@timestamp":"2026-06-22T10:02:00Z","message":"ok","level":"INFO"}}
			]
		}
	}`
	result, err := ParseSearchResult(strings.NewReader(esJSON), 500, nil, "@timestamp")
	require.NoError(t, err)
	assert.Equal(t, int64(3), result.TotalHits)
	assert.True(t, result.IsExhaustive)
	assert.Equal(t, 3, result.ReturnedHits)
	assert.Equal(t, int64(42), result.TookMs)
	assert.Equal(t, "error 1", result.Entries[0].Fields["message"].Text)
	assert.Equal(t, "ERROR", result.Entries[0].Fields["level"].Text)
	assert.False(t, result.Entries[0].Timestamp.IsZero())
}

func TestParseSearchResult_GteRelation(t *testing.T) {
	esJSON := `{
		"took": 10,
		"hits": {
			"total": {"value": 10000, "relation": "gte"},
			"hits": [{"_id":"1","_source":{"@timestamp":"2026-06-22T10:00:00Z","message":"test"}}]
		}
	}`
	result, err := ParseSearchResult(strings.NewReader(esJSON), 0, nil, "@timestamp")
	require.NoError(t, err)
	assert.False(t, result.IsExhaustive)
}

func TestParseSearchResult_Truncation(t *testing.T) {
	esJSON := `{
		"took": 1,
		"hits": {
			"total": {"value": 1, "relation": "eq"},
			"hits": [{"_id":"1","_source":{"@timestamp":"2026-06-22T10:00:00Z","message":"abcdefghijklmnopqrstuvwxyz"}}]
		}
	}`
	result, err := ParseSearchResult(strings.NewReader(esJSON), 10, nil, "@timestamp")
	require.NoError(t, err)
	fv := result.Entries[0].Fields["message"]
	assert.True(t, fv.Truncated)
	assert.Equal(t, 10, len(fv.Text))
	assert.Equal(t, 26, fv.FullLength)
}

func TestParseSearchResult_ExpandFieldsNotTruncated(t *testing.T) {
	esJSON := `{
		"took": 1,
		"hits": {
			"total": {"value": 1, "relation": "eq"},
			"hits": [{"_id":"1","_source":{"@timestamp":"2026-06-22T10:00:00Z","message":"abcdefghijklmnopqrstuvwxyz"}}]
		}
	}`
	result, err := ParseSearchResult(strings.NewReader(esJSON), 5, []string{"message"}, "@timestamp")
	require.NoError(t, err)
	fv := result.Entries[0].Fields["message"]
	assert.False(t, fv.Truncated)
	assert.Equal(t, 26, len(fv.Text))
}

// ==========================================================================
// ParseStatsResult 测试
// ==========================================================================

func TestParseStatsResult(t *testing.T) {
	esJSON := `{
		"aggregations": {
			"by_field": {
				"sum_other_doc_count": 2,
				"buckets": [
					{"key": "ERROR", "doc_count": 120},
					{"key": "WARN", "doc_count": 45}
				]
			}
		}
	}`
	result, err := ParseStatsResult(strings.NewReader(esJSON), "level")
	require.NoError(t, err)
	assert.Equal(t, "level", result.Field)
	assert.Len(t, result.Buckets, 2)
	assert.Equal(t, "ERROR", result.Buckets[0].Value)
	assert.Equal(t, int64(120), result.Buckets[0].Count)
}

// ==========================================================================
// ParseDocResponse 测试
// ==========================================================================

func TestParseDocResponse(t *testing.T) {
	esJSON := `{"_id":"abc123","found":true,"_source":{"@timestamp":"2026-06-22T10:00:00Z","message":"full stack trace here...","level":"ERROR"}}`
	entry, err := ParseDocResponse(strings.NewReader(esJSON), "@timestamp")
	require.NoError(t, err)
	assert.Equal(t, "abc123", entry.ID)
	assert.Equal(t, "ERROR", entry.Fields["level"].Text)
	assert.False(t, entry.Timestamp.IsZero())
}

func TestParseDocResponse_NotFound(t *testing.T) {
	esJSON := `{"_id":"missing","found":false}`
	_, err := ParseDocResponse(strings.NewReader(esJSON), "@timestamp")
	assert.Error(t, err)
}

// ==========================================================================
// ExtractSourceInfo 测试
// ==========================================================================

func TestExtractSourceInfo(t *testing.T) {
	mappingJSON := `{
		"logs": {
			"mappings": {
				"properties": {
					"@timestamp": {"type": "date"},
					"message": {"type": "text"},
					"level": {"type": "keyword"},
					"host": {"type": "keyword"},
					"trace_id": {"type": "keyword"}
				}
			}
		}
	}`
	info, err := ExtractSourceInfo("prod-logs", "logs-*", strings.NewReader(mappingJSON), 5000000, "@timestamp", "message")
	require.NoError(t, err)
	assert.Equal(t, "prod-logs", info.Name)
	assert.Equal(t, "elasticsearch", info.Backend)
	assert.Equal(t, int64(5000000), info.Estimated)
	assert.GreaterOrEqual(t, len(info.Fields), 5)

	// 验证角色标注
	traceField := findField(info.Fields, "trace_id")
	require.NotNil(t, traceField)
	assert.True(t, traceField.HasRole("trace_id"))
}

func findField(fields []logprobe.FieldInfo, name string) *logprobe.FieldInfo {
	for _, f := range fields {
		if f.Name == name {
			return &f
		}
	}
	return nil
}

// ==========================================================================
// JSON 序列化验证（确保 body 可以完整序列化为 ES 接受的 JSON）
// ==========================================================================

func TestBodyIsJSONSerializable(t *testing.T) {
	req := logprobe.SearchRequest{
		Query:     "test",
		TimeRange: logprobe.TimeRange{From: time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)},
		Limit:     50,
		Fields:    []string{"@timestamp", "message"},
	}
	body := BuildSearchBody(req, "logs-*")
	b, err := json.Marshal(body)
	require.NoError(t, err)

	var reform map[string]any
	require.NoError(t, json.NewDecoder(bytes.NewReader(b)).Decode(&reform))
	assert.Contains(t, reform, "query")
	assert.Contains(t, reform, "size")
}

func TestBuildSearchBody_FieldTruncation(t *testing.T) {
	req := logprobe.SearchRequest{
		Query:        "error",
		Fields:       []string{"@timestamp", "level", "message"},
		FieldMaxLen:  100,
		ExpandFields: []string{"@timestamp"}, // @timestamp 不截断
		Limit:        10,
	}
	body := BuildSearchBody(req, "logs-*")

	// _source 只应包含白名单字段（@timestamp），message 和 level 走 script_fields
	source := body["_source"].([]string)
	assert.Equal(t, []string{"@timestamp"}, source)

	// script_fields 应包含 message 和 level
	sf := body["script_fields"].(map[string]any)
	assert.Contains(t, sf, "message")
	assert.Contains(t, sf, "level")

	// message 的 script 应包含 substring 逻辑
	msgScript := sf["message"].(map[string]any)["script"].(map[string]any)
	assert.Contains(t, msgScript["source"].(string), "substring")
	assert.Equal(t, "painless", msgScript["lang"])
}

func TestBuildSearchBody_NoTruncationWhenFieldMaxLenZero(t *testing.T) {
	req := logprobe.SearchRequest{
		Query:       "error",
		Fields:      []string{"@timestamp", "message"},
		FieldMaxLen: 0, // 不截断
		Limit:       10,
	}
	body := BuildSearchBody(req, "logs-*")

	// 不用 script_fields，全部走 _source
	assert.Equal(t, []string{"@timestamp", "message"}, body["_source"].([]string))
	assert.NotContains(t, body, "script_fields")
}

func TestParseScriptFieldValue_Structured(t *testing.T) {
	// 模拟 ES 返回的结构化截断对象
	val := map[string]any{
		"text":      "truncated text...",
		"len":       float64(636),
		"truncated": true,
	}
	fv := parseScriptFieldValue(val)
	assert.Equal(t, "truncated text...", fv.Text)
	assert.Equal(t, 636, fv.FullLength)
	assert.True(t, fv.Truncated)
}

func TestParseScriptFieldValue_Plain(t *testing.T) {
	// 普通字符串（用户自定义 script_field）
	fv := parseScriptFieldValue("hello world")
	assert.Equal(t, "hello world", fv.Text)
	assert.Equal(t, len("hello world"), fv.FullLength)
	assert.False(t, fv.Truncated)
}

func TestParseScriptFieldValue_Nil(t *testing.T) {
	fv := parseScriptFieldValue(nil)
	assert.Empty(t, fv.Text)
	assert.Equal(t, 0, fv.FullLength)
}

func TestSplitFieldsForTruncation(t *testing.T) {
	expandSet := map[string]bool{"@timestamp": true}
	sourceFields, scriptFields := splitFieldsForTruncation(
		[]string{"@timestamp", "level", "message"},
		expandSet,
		100,
	)

	// @timestamp 在白名单 → _source
	assert.Equal(t, []string{"@timestamp"}, sourceFields)

	// level 和 message → script_fields
	assert.Len(t, scriptFields, 2)
	assert.Contains(t, scriptFields, "level")
	assert.Contains(t, scriptFields, "message")

	// 验证 message 的 script 结构
	msgSF := scriptFields["message"].(map[string]any)
	msgScript := msgSF["script"].(map[string]any)
	assert.Equal(t, "painless", msgScript["lang"])
	assert.Contains(t, msgScript["source"].(string), "substring")
	assert.Contains(t, msgScript["source"].(string), "message")
}

func TestSplitFieldsForTruncation_AllExpand(t *testing.T) {
	expandSet := map[string]bool{"@timestamp": true, "message": true}
	sourceFields, scriptFields := splitFieldsForTruncation(
		[]string{"@timestamp", "message", "level"},
		expandSet,
		100,
	)

	// @timestamp + message 在白名单 → _source；level 不在白名单 → script_fields
	assert.Equal(t, []string{"@timestamp", "message"}, sourceFields)
	assert.Contains(t, scriptFields, "level")
}

func TestBuildSearchBody_AllScriptFieldsSourceFalse(t *testing.T) {
	// 无 ExpandFields，但 @timestamp 始终强制走 _source（时间戳提取需要）
	// level 和 message 走 script_fields
	req := logprobe.SearchRequest{
		Query:        "error",
		Fields:       []string{"@timestamp", "level", "message"},
		FieldMaxLen:  100,
		ExpandFields: nil, // 无白名单，全部截断
		Limit:        10,
	}
	body := BuildSearchBody(req, "logs-*")

	// @timestamp 始终在 _source（提取时间戳需要）
	assert.Equal(t, []string{"@timestamp"}, body["_source"])

	// level 和 message 在 script_fields
	sf := body["script_fields"].(map[string]any)
	assert.Len(t, sf, 2)
	assert.Contains(t, sf, "level")
	assert.Contains(t, sf, "message")
}
