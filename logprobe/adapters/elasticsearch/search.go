package elasticsearch

import (
	"fmt"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logprobe"
	"github.com/izliang/bore/logprobe/adapters/escommon"
)

// Search 执行关键词搜索。
func (s *Source) Search(atx appctx.AppContext, req logprobe.SearchRequest) (*logprobe.SearchResult, error) {
	bodyMap := escommon.BuildSearchBody(req, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.doSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Search: %w", err)
	}
	defer respBody.Close()
	return escommon.ParseSearchResult(respBody, req.FieldMaxLen, req.ExpandFields, s.cfg.TimeField)
}

// Stats 执行字段分布统计。
func (s *Source) Stats(atx appctx.AppContext, req logprobe.StatsRequest) (*logprobe.StatsResult, error) {
	bodyMap := escommon.BuildStatsBody(req, s.cfg.Index, s.searchOpts()...)
	respBody, err := s.doSearch(atx, bodyMap)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch: Stats: %w", err)
	}
	defer respBody.Close()
	return escommon.ParseStatsResult(respBody, req.Field)
}
