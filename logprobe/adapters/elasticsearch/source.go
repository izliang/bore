// Package elasticsearch 实现 logprobe.LogSource 接口，直连 Elasticsearch。
//
// 复用 escommon 包的查询构建与响应解析逻辑。
// 传输层基于 go-elasticsearch/v8 官方客户端。
//
// # 连接
//
//	esCfg := elasticsearch.Config{Addresses: []string{"http://localhost:9200"}}
//	client, _ := elasticsearch.NewClient(esCfg)
//	src := elasticsearch.NewSource(client, elasticsearch.Config{
//	    Name:  "prod-logs",
//	    Index: "applog-*",
//	})
//
// # 注册到 Registry
//
//	reg := logprobe.NewRegistry()
//	reg.Register(src)
//	result, _ := src.Search(ctx, logprobe.SearchRequest{Query: "ERROR"})
package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/izliang/bore/appctx"

	"github.com/izliang/bore/logprobe"
	"github.com/izliang/bore/logprobe/adapters/escommon"
)

// ==========================================================================
// Config
// ==========================================================================

// Config 配置 ES 数据源。
type Config struct {
	// Name 数据源的人类可读名称（如 "prod-app-logs"）。
	// 同时作为 LogSource.ID() 的返回值。
	Name string

	// Index ES 索引模式（如 "applog-*"、"logs-2026.*"）。
	// 支持通配符和多索引（逗号分隔）。
	Index string

	// TimeField 时间戳字段名。默认 "@timestamp"。
	TimeField string

	// MessageField 消息主体字段名。默认 "message"。
	MessageField string

	// SearchFields query_string 搜索的目标字段。空 = 搜索所有字段。
	SearchFields []string

	// ScriptFields 注入每次 _search 请求的 script_fields。
	// 例如：{"message_preview": {"script": {"source": "..."}}}。
	// 注意：script_fields 的值不可通过 Expand 展开完整内容。
	ScriptFields map[string]any
}

func (c *Config) normalize() {
	if c.TimeField == "" {
		c.TimeField = "@timestamp"
	}
	if c.MessageField == "" {
		c.MessageField = "message"
	}
}

// ==========================================================================
// Source
// ==========================================================================

// Source 实现 logprobe.LogSource，直连 Elasticsearch。
type Source struct {
	client *elasticsearch.Client
	cfg    Config
	cursor logprobe.Cursor
}

// NewSource 创建一个 ES 数据源。
//
// client 是已配置好的 elasticsearch.Client。
func NewSource(client *elasticsearch.Client, cfg Config) *Source {
	cfg.normalize()
	return &Source{client: client, cfg: cfg, cursor: logprobe.Cursor{Backend: "elasticsearch"}}
}

// ID 返回数据源名称。
func (s *Source) ID() string { return s.cfg.Name }

// Cursor 返回当前流式位置。
func (s *Source) Cursor() logprobe.Cursor { return s.cursor }

// SetCursor 恢复流式位置。
func (s *Source) SetCursor(c logprobe.Cursor) { s.cursor = c }

// ==========================================================================
// Info — 数据源自描述
// ==========================================================================

// Info 获取数据源的字段结构和预估规模。
//
// 字段发现策略（按优先级）：
//  1. _field_caps?fields=* — 权限更宽松，直接提供 searchable/aggregatable
//  2. _mapping — 回退方案
func (s *Source) Info(atx appctx.AppContext) (*logprobe.SourceInfo, error) {
	// 获取文档总数
	countResp, err := s.doEsapi(atx, func() (*esapi.Response, error) {
		return s.client.Count(
			s.client.Count.WithIndex(s.cfg.Index),
			s.client.Count.WithContext(atx),
		)
	})
	var estimated int64
	if err == nil && countResp != nil {
		defer countResp.Body.Close()
		var cr struct {
			Count int64 `json:"count"`
		}
		if json.NewDecoder(countResp.Body).Decode(&cr) == nil {
			estimated = cr.Count
		}
	}

	// 优先 _field_caps
	fcResp, fcErr := s.doEsapi(atx, func() (*esapi.Response, error) {
		return s.client.FieldCaps(
			s.client.FieldCaps.WithIndex(s.cfg.Index),
			s.client.FieldCaps.WithFields("*"),
			s.client.FieldCaps.WithContext(atx),
		)
	})
	if fcErr == nil {
		defer fcResp.Body.Close()
		info, infoErr := escommon.ExtractSourceInfoFromFieldCaps(
			s.cfg.Name, s.cfg.Index, fcResp.Body, estimated,
			s.cfg.TimeField, s.cfg.MessageField,
		)
		if len(info.Fields) > 0 {
			return info, infoErr
		}
	}

	// 回退 _mapping
	mappingResp, err := s.doEsapi(atx, func() (*esapi.Response, error) {
		return s.client.Indices.GetMapping(
			s.client.Indices.GetMapping.WithIndex(s.cfg.Index),
			s.client.Indices.GetMapping.WithContext(atx),
		)
	})
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Info: 获取 mapping 失败: %w", err)
	}
	defer mappingResp.Body.Close()
	return escommon.ExtractSourceInfo(
		s.cfg.Name, s.cfg.Index, mappingResp.Body, estimated,
		s.cfg.TimeField, s.cfg.MessageField,
	)
}

// ==========================================================================
// doEsapi — esapi 请求封装
// ==========================================================================

// doEsapi 执行 esapi 请求，统一处理错误。
// fn 返回原始响应，调用方负责关闭 Body。
func (s *Source) doEsapi(ctx context.Context, fn func() (*esapi.Response, error)) (*esapi.Response, error) {
	resp, err := fn()
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: 请求失败: %w", err)
	}
	if resp.IsError() {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("elasticsearch: ES 返回错误 (%d): %s", resp.StatusCode, body)
	}
	return resp, nil
}

// doSearch 执行 _search 请求，传入已构建的 body map。
func (s *Source) doSearch(ctx context.Context, body map[string]any) (io.ReadCloser, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: JSON 序列化失败: %w", err)
	}
	resp, err := s.doEsapi(ctx, func() (*esapi.Response, error) {
		return s.client.Search(
			s.client.Search.WithIndex(s.cfg.Index),
			s.client.Search.WithBody(bytes.NewReader(jsonBody)),
			s.client.Search.WithContext(ctx),
		)
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// doGet 执行 GET /{index}/_doc/{id}?xxx 请求。
func (s *Source) doGet(ctx context.Context, docID string, sourceFields []string) (io.ReadCloser, error) {
	opts := []func(*esapi.GetRequest){
		s.client.Get.WithContext(ctx),
	}
	if len(sourceFields) > 0 {
		opts = append(opts, s.client.Get.WithSourceIncludes(sourceFields...))
	}
	resp, err := s.doEsapi(ctx, func() (*esapi.Response, error) {
		return s.client.Get(s.cfg.Index, docID, opts...)
	})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// ==========================================================================
// searchOpts
// ==========================================================================

func (s *Source) searchOpts() []escommon.SearchOption {
	var opts []escommon.SearchOption
	if s.cfg.TimeField != "@timestamp" {
		opts = append(opts, escommon.WithTimeField(s.cfg.TimeField))
	}
	if len(s.cfg.SearchFields) > 0 {
		opts = append(opts, escommon.WithSearchFields(s.cfg.SearchFields...))
	}
	if len(s.cfg.ScriptFields) > 0 {
		opts = append(opts, escommon.WithScriptFields(s.cfg.ScriptFields))
	}
	return opts
}
