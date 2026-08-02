package kibanaproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logprobe"
)

// ==========================================================================
// Stream — 游标流式消费
// ==========================================================================

// Stream 创建基于 search_after + PIT 的流式迭代器。
//
// 与 elasticsearch 适配器一样使用 search_after + PIT 模式，
// 只是传输层走 Kibana Proxy。
func (s *Source) Stream(atx appctx.AppContext, req logprobe.StreamRequest) (*logprobe.LogCursor, error) {
	pitID, err := s.createPIT(atx)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Stream: 创建 PIT 失败: %w", err)
	}

	state := &kpStreamState{
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

// kpStreamState 流式游标的内部状态。
type kpStreamState struct {
	src        *Source
	req        logprobe.StreamRequest
	pitID      string
	sortValues []any
	follow     bool
	done       bool
}

func (st *kpStreamState) next(ctx context.Context) ([]logprobe.LogEntry, error) {
	if st.done {
		return nil, logprobe.ErrStreamDone
	}

	body := st.buildStreamBody()
	jsonBody, _ := json.Marshal(body)
	respBody, err := st.src.doRequest(ctx, "POST", st.src.cfg.Index+"/_search", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	defer respBody.Close()

	entries, sortVals, err := st.parseHits(respBody)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		if !st.follow {
			st.done = true
			return nil, logprobe.ErrStreamDone
		}
		return nil, nil
	}

	if len(sortVals) > 0 {
		st.sortValues = sortVals
	}
	return entries, nil
}

func (st *kpStreamState) cursor() logprobe.Cursor {
	if len(st.sortValues) == 0 {
		return logprobe.Cursor{Backend: st.src.ID()}
	}
	pos, _ := json.Marshal(map[string]any{
		"pit":         st.pitID,
		"sort_values": st.sortValues,
	})
	return logprobe.Cursor{Backend: st.src.ID(), Position: pos}
}

func (st *kpStreamState) close() error {
	body := map[string]any{"id": st.pitID}
	jsonBody, _ := json.Marshal(body)
	respBody, err := st.src.doRequest(appctx.New(), "DELETE", "_pit", bytes.NewReader(jsonBody))
	if err != nil {
		return err
	}
	if respBody != nil {
		respBody.Close()
	}
	return nil
}

func (st *kpStreamState) buildStreamBody() map[string]any {
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

	filters := make([]any, 0)
	if st.req.Query != "" && st.req.Query != "*" {
		filters = append(filters, map[string]any{
			"query_string": map[string]any{
				"query": st.req.Query, "default_operator": "AND", "lenient": true,
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
			filters = append(filters, map[string]any{"range": map[string]any{st.src.cfg.TimeField: r}})
		}
	}
	if len(filters) > 0 {
		body["query"] = map[string]any{"bool": map[string]any{"filter": filters}}
	}

	if len(st.sortValues) > 0 {
		body["search_after"] = st.sortValues
	}
	return body
}

func (st *kpStreamState) parseHits(body io.Reader) ([]logprobe.LogEntry, []any, error) {
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

// createPIT 通过 Kibana Proxy 创建 Point In Time。
func (s *Source) createPIT(atx appctx.AppContext) (string, error) {
	path := s.cfg.Index + "/_pit?keep_alive=5m"
	respBody, err := s.doRequest(atx, "POST", path, nil)
	if err != nil {
		return "", err
	}
	defer respBody.Close()
	var pitResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(respBody).Decode(&pitResp); err != nil {
		return "", fmt.Errorf("解析 PIT 响应失败: %w", err)
	}
	return pitResp.ID, nil
}
