package elasticsearch

import (
	"fmt"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logprobe"
	"github.com/izliang/bore/logprobe/adapters/escommon"
)

// Context 获取指定日志条目的上下文。
func (s *Source) Context(atx appctx.AppContext, req logprobe.ContextRequest) (*logprobe.SearchResult, error) {
	// 先获取参考条目的完整信息
	refBody, err := s.doGet(atx, req.EntryID, nil)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Context: 获取参考条目失败: %w", err)
	}
	defer refBody.Close()
	refEntry, err := escommon.ParseDocResponse(refBody, s.cfg.TimeField)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Context: 解析参考条目失败: %w", err)
	}

	// 构建上下文查询
	bodyMap := escommon.BuildContextBody(req, *refEntry, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.doSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Context: %w", err)
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
		result := logprobe.ExpandResult{
			EntryID: t.EntryID,
			Field:   t.Field,
		}
		respBody, err := s.doGet(atx, t.EntryID, []string{t.Field})
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}
		entry, err := escommon.ParseDocResponse(respBody, s.cfg.TimeField)
		respBody.Close()
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
