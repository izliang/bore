package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/izliang/bore/appctx"

	"github.com/izliang/bore/logprobe"
)

// ==========================================================================
// Stream — 游标流式消费
// ==========================================================================

// Stream 创建基于 search_after + PIT 的流式迭代器。
//
// 实现策略：
//  1. 创建 Point In Time (PIT)，保持时间点一致性
//  2. 首次搜索不传 search_after，获取第一批数据
//  3. 后续搜索传 search_after=[上批最后一条的 sort values]
//  4. Follow=true 时持续轮询
//  5. LogCursor.Close() 释放 PIT
func (s *Source) Stream(atx appctx.AppContext, req logprobe.StreamRequest) (*logprobe.LogCursor, error) {
	// 创建 PIT
	pitID, err := s.createPIT(atx)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Stream: 创建 PIT 失败: %w", err)
	}

	state := &streamState{
		src:        s,
		req:        req,
		pitID:      pitID,
		sortValues: nil,
		follow:     req.Follow,
	}

	return &logprobe.LogCursor{
		Next:   state.next,
		Cursor: state.cursor,
		Close:  state.close,
	}, nil
}

// ==========================================================================
// streamState — 游标内部状态
// ==========================================================================

type streamState struct {
	src        *Source
	req        logprobe.StreamRequest
	pitID      string
	sortValues []any // 上次最后一条的 sort values，nil = 首次
	follow     bool
	done       bool
}

func (st *streamState) next(ctx context.Context) ([]logprobe.LogEntry, error) {
	if st.done {
		return nil, logprobe.ErrStreamDone
	}

	body := st.buildStreamBody()
	respBody, err := st.src.doSearch(ctx, body)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	// 直接解析 hits 提取 entries + sort values
	entries, sortVals, err := st.parseHits(respBody)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		if !st.follow {
			st.done = true
			return nil, logprobe.ErrStreamDone
		}
		// Follow 模式：暂无新数据，返回空批次
		return nil, nil
	}

	// 更新游标
	if len(sortVals) > 0 {
		st.sortValues = sortVals
	}

	return entries, nil
}

func (st *streamState) cursor() logprobe.Cursor {
	if len(st.sortValues) == 0 {
		return logprobe.Cursor{Backend: st.src.ID()}
	}
	pos, _ := json.Marshal(map[string]any{
		"pit":         st.pitID,
		"sort_values": st.sortValues,
	})
	return logprobe.Cursor{
		Backend:  st.src.ID(),
		Position: pos,
	}
}

func (st *streamState) close() error {
	return st.src.deletePIT(context.Background(), st.pitID)
}

// buildStreamBody 构建 search_after 请求体。
func (st *streamState) buildStreamBody() map[string]any {
	limit := st.req.Limit
	if limit <= 0 {
		limit = 500
	}

	body := map[string]any{
		"size": limit,
		"pit": map[string]any{
			"id":         st.pitID,
			"keep_alive": "5m",
		},
		"sort": []any{
			map[string]any{st.src.cfg.TimeField: "asc"},
			map[string]any{"_shard_doc": "asc"},
		},
		"track_total_hits": false,
	}

	// 添加过滤条件
	filters := make([]any, 0)
	if st.req.Query != "" && st.req.Query != "*" {
		filters = append(filters, map[string]any{
			"query_string": map[string]any{
				"query":            st.req.Query,
				"default_operator": "AND",
				"lenient":          true,
			},
		})
	}
	if !st.req.TimeRange.IsZero() {
		r := map[string]any{}
		if !st.req.TimeRange.From.IsZero() {
			r["gte"] = st.req.TimeRange.From.Format(time.RFC3339Nano)
		}
		if !st.req.TimeRange.To.IsZero() {
			r["lte"] = st.req.TimeRange.To.Format(time.RFC3339Nano)
		}
		if len(r) > 0 {
			filters = append(filters, map[string]any{
				"range": map[string]any{st.src.cfg.TimeField: r},
			})
		}
	}
	if len(filters) > 0 {
		body["query"] = map[string]any{
			"bool": map[string]any{"filter": filters},
		}
	}

	// 非首次：加入 search_after
	if len(st.sortValues) > 0 {
		body["search_after"] = st.sortValues
	}

	return body
}

// parseHits 从 _search 响应中提取 entries 和最后一条的 sort values。
func (st *streamState) parseHits(body io.Reader) ([]logprobe.LogEntry, []any, error) {
	var resp struct {
		Hits struct {
			Hits []struct {
				ID     string          `json:"_id"`
				Source json.RawMessage `json:"_source"`
				Sort   []any           `json:"sort"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		return nil, nil, err
	}

	entries := make([]logprobe.LogEntry, 0, len(resp.Hits.Hits))
	var lastSort []any
	for _, hit := range resp.Hits.Hits {
		var src map[string]any
		json.Unmarshal(hit.Source, &src)
		entry := logprobe.LogEntry{
			ID:     hit.ID,
			Fields: make(map[string]logprobe.FieldValue),
			Raw:    hit.Source,
		}
		for k, v := range src {
			text := fmt.Sprint(v)
			entry.Fields[k] = logprobe.FieldValue{Text: text, FullLength: len(text)}
			if k == st.src.cfg.TimeField {
				if t, err := time.Parse(time.RFC3339Nano, text); err == nil {
					entry.Timestamp = t
				}
			}
		}
		entries = append(entries, entry)
		lastSort = hit.Sort
	}
	return entries, lastSort, nil
}

// ==========================================================================
// PIT 生命周期
// ==========================================================================

// createPIT 创建 Point In Time。
func (s *Source) createPIT(ctx context.Context) (string, error) {
	resp, err := s.doEsapi(ctx, func() (*esapi.Response, error) {
		return s.client.OpenPointInTime([]string{s.cfg.Index}, "5m", s.client.OpenPointInTime.WithContext(ctx))
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var pitResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pitResp); err != nil {
		return "", fmt.Errorf("解析 PIT 响应失败: %w", err)
	}
	return pitResp.ID, nil
}

// deletePIT 释放 Point In Time。
func (s *Source) deletePIT(ctx context.Context, pitID string) error {
	body := map[string]any{"id": pitID}
	jsonBody, _ := json.Marshal(body)
	resp, err := s.doEsapi(ctx, func() (*esapi.Response, error) {
		return s.client.ClosePointInTime(s.client.ClosePointInTime.WithBody(bytes.NewReader(jsonBody)), s.client.ClosePointInTime.WithContext(ctx))
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
