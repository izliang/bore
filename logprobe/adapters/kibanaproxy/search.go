package kibanaproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logprobe"
	"github.com/izliang/bore/logprobe/adapters/escommon"
)

// Search 执行关键词搜索。
func (s *Source) Search(atx appctx.AppContext, req logprobe.SearchRequest) (*logprobe.SearchResult, error) {
	bodyMap := escommon.BuildSearchBody(req, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.sendSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Search: %w", err)
	}
	defer respBody.Close()
	return escommon.ParseSearchResult(respBody, req.FieldMaxLen, req.ExpandFields, s.cfg.TimeField)
}

// Stats 执行字段分布统计。
func (s *Source) Stats(atx appctx.AppContext, req logprobe.StatsRequest) (*logprobe.StatsResult, error) {
	bodyMap := escommon.BuildStatsBody(req, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.sendSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Stats: %w", err)
	}
	defer respBody.Close()
	return escommon.ParseStatsResult(respBody, req.Field)
}

// Context 获取指定日志条目的上下文。
func (s *Source) Context(atx appctx.AppContext, req logprobe.ContextRequest) (*logprobe.SearchResult, error) {
	refEntry, err := s.getDoc(atx, req.EntryID)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Context: 获取参考条目失败: %w", err)
	}
	bodyMap := escommon.BuildContextBody(req, *refEntry, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.sendSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Context: %w", err)
	}
	defer respBody.Close()
	return escommon.ParseSearchResult(respBody, 0, nil, s.cfg.TimeField)
}

// Expand 批量获取截断字段的完整内容。
func (s *Source) Expand(atx appctx.AppContext, targets []logprobe.ExpandTarget) ([]logprobe.ExpandResult, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	results := make([]logprobe.ExpandResult, 0, len(targets))
	for _, t := range targets {
		result := logprobe.ExpandResult{EntryID: t.EntryID, Field: t.Field}
		entry, err := s.getDoc(atx, t.EntryID)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		if fv, ok := entry.Fields[t.Field]; ok {
			val, expandErr := fv.Expand(atx)
			if expandErr != nil {
				result.Error = expandErr.Error()
			} else {
				result.Value = val
			}
		} else {
			result.Error = fmt.Sprintf("字段 %s 在文档 %s 中不存在", t.Field, t.EntryID)
		}
		results = append(results, result)
	}
	return results, nil
}

// Info 获取数据源自描述（AI 第一步调用此方法了解字段结构）。
//
// 字段发现策略（按优先级）：
//  1. _field_caps?fields=* — 直接提供 searchable/aggregatable 元数据，权限更宽松
//  2. _mapping — 回退方案
//
// 任意一个成功且返回了字段列表即停止。
func (s *Source) Info(atx appctx.AppContext) (*logprobe.SourceInfo, error) {
	// 获取预估文档数
	countBody, err := s.doRequest(atx, "GET", s.cfg.Index+"/_count", nil)
	var estimated int64
	if err == nil {
		defer countBody.Close()
		var cr struct{ Count int64 }
		if json.NewDecoder(countBody).Decode(&cr) == nil {
			estimated = cr.Count
		}
	}

	// 优先 _field_caps — 权限更宽松，直接提供 searchable/aggregatable
	fcBody, fcErr := s.doRequest(atx, "GET", s.cfg.Index+"/_field_caps?fields=*", nil)
	if fcErr == nil {
		defer fcBody.Close()
		info, infoErr := escommon.ExtractSourceInfoFromFieldCaps(
			s.cfg.Name, s.cfg.Index, fcBody, estimated,
			s.cfg.TimeField, s.cfg.MessageField,
		)
		if len(info.Fields) > 0 {
			return info, infoErr
		}
	}

	// 回退 _mapping
	mappingBody, err := s.doRequest(atx, "GET", s.cfg.Index+"/_mapping", nil)
	if err != nil {
		return nil, fmt.Errorf("kibanaproxy: Info: 获取 mapping 失败: %w", err)
	}
	defer mappingBody.Close()
	return escommon.ExtractSourceInfo(
		s.cfg.Name, s.cfg.Index, mappingBody, estimated,
		s.cfg.TimeField, s.cfg.MessageField,
	)
}

// ==========================================================================
// 内部方法
// ==========================================================================

func (s *Source) sendSearch(ctx context.Context, body map[string]any) (io.ReadCloser, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("JSON 序列化失败: %w", err)
	}
	return s.doRequest(ctx, "POST", s.cfg.Index+"/_search", bytes.NewReader(jsonBody))
}

func (s *Source) getDoc(ctx context.Context, docID string) (*logprobe.LogEntry, error) {
	respBody, err := s.doRequest(ctx, "GET", s.cfg.Index+"/_doc/"+docID, nil)
	if err != nil {
		return nil, err
	}
	defer respBody.Close()
	return escommon.ParseDocResponse(respBody, s.cfg.TimeField)
}
