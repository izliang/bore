package kibanaproxy

import (
	"os"
	"testing"
	"time"

	"github.com/izliang/bore/appctx"
	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/logprobe"
)

func testInit(atx appctx.AppContext) logprobe.LogSource {
	//adapter := NewSource(Config{
	//	Name:          "prod-app-logs",
	//	KibanaBaseURL: "https://kibana-zw..cn",
	//	Index:         "applog-prod-internal-*",
	//	Cookies:       "sid=Fe26.2**77b2f01bb970286828111e1f6aa4e491ab641559e5af6ced132b23621b085236*I3ebwD90JHUBapLxOklB7A*wEV4pdylaeP9kULyQ_oMnyE-MW9NaHxphRXwxgEw7BZfKtcevIDPpnDfO7E5qtDYCl0aNd3wW5VE7ZIPvfRAbR412VZStDUUnBCPMPmxqfSwl1auRI34BQI0arD6A9xWFMRvTlOcL9r-tcKLxhbNRAP6INswBVnpfS9xiR_NO3NiOc355XwYmBUIb9CtHV5ww_AWecrtvedTQKDjkFvLUlKwPH2LGf1x78Qmqwd3r6Q**1db619d655035d8da4fa111f2d54aab9a4c05a0a92bde35686738e179c6e843c*og3b3GApgsNWZZElsIpXIY5e7b_g85mZ5HNo-HevJM8;",
	//	// Logger HTTP 请求日志
	//	Logger: atx.LogitLogger(),
	//	// MaxRetries 网络错误自动重试
	//	MaxRetries: 2,
	//	// 不设置 SearchFields，让 ES 跨所有字段搜索
	//})
	adapter := NewSource(Config{
		Name:          "pre-app-logs",
		KibanaBaseURL: "https://kibana-yz..cn",
		Index:         "applog-yufa-20*",
		Cookies:       "sid=Fe26.2**59357cf233abf40b5a9e3f9c433b92273cc98e5b13f215850f67b66da21a45f1*h7H4RlVc-KA2vTIPu3BZ8Q*ZK75CviE1iRqKv7VKqunXr8LHIa-N3eRP6pRAFv8pjLnVodkyOpj6kKnCovPLQSlV8-uiO01bJyJL2nZu6nIDPoQ91t97nWoQeF3Gu1st2Bvkd69kpWuJpn4bVIEJpsy0tcBbNEMXqonmUnlP-UaFwTmTH_Lu4Xiwi2RXTfiWRGFWJI-vy_B__aREbxqUpsPMeOI6ROwjiQj3b1aQp-g-OxdpIxfEf8ZLw8LdaPPR7A**26edfd9e87d08a59992659ff09ddd3a88cacef5485a87800c91ee5a6c41fd39b*EqHhbABCmqZlGSvEn9QackY9oUlIqDDP7CgmVzFh4lM; mnrhuwtfpcxw=eyJ0eXAiOiJKV1QiLCJhbGciOiJFUzI1NiJ9.eyJleHAiOjE3ODI5OTY3MjIsImlhdCI6MTc4Mjk4NTkyMiwiaXNzIjoib3BzZGV2Iiwic3ViIjoiY21kYiIsIm5iZiI6MTc4Mjk4NTkyMiwidXNlcl9pZCI6NTg2NywidXNlcm5hbWUiOiJsaWFuZy56aGFvIiwiZW1haWwiOiJsaWFuZy56aGFvQDEwMGNyZWRpdC5jb20iLCJkYXRhIjp7InVzZXJuYW1lIjoibGlhbmcuemhhbyIsImxvZ2luX3RpbWUiOjE3ODI5ODU5MjIsImV4cCI6MTc4Mjk5NjcyMn19.r3zO6QglhC_3mXJXsQt4ELZZ-Fw1JIjeBrr_m0nNAQRcT3jDX1Eaatcyo15p-24HuzCynvO6lL6E_S06cBrdhw",
		// Logger HTTP 请求日志
		Logger: atx.LogitLogger(),
		// MaxRetries 网络错误自动重试
		MaxRetries: 2,
		// 不设置 SearchFields，让 ES 跨所有字段搜索
	})
	return adapter
}

// TestInfo 验证 Info 能正确获取 ES mapping 并返回 SourceInfo。
// 需要有效的 Kibana session cookie 和网络连接。
func TestInfo(t *testing.T) {
	atx := appctx.New(
		appctx.WithLogger(logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
			Level: logit.Debug,
		})),
	)

	adapter := testInit(atx)

	// Step 1: AI 获取数据源自描述 — 了解有哪些字段可查询
	info, err := adapter.Info(atx)
	if err != nil {
		t.Fatal("Info 失败:", err)
	}
	t.Logf("数据源: %s, 后端: %s, 预估文档数: %d", info.Name, info.Backend, info.Estimated)
	t.Logf("时间字段: %s, 消息字段: %s", info.TimeField, info.MessageField)
	t.Logf("字段列表 (%d 个):", len(info.Fields))
	for _, f := range info.Fields {
		t.Logf("  %-30s type=%-10s searchable=%-5v roles=%v", f.Name, f.Type, f.Searchable, f.Roles)
	}

	// Step 2: AI 基于字段信息构造搜索
	_ = adapter
	_ = logprobe.SearchRequest{}
}

func Test_Search(t *testing.T) {
	atx := appctx.New(
		appctx.WithLogger(logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
			Level: logit.Debug,
		})),
	)

	adapter := testInit(atx)

	// 构造搜索——使用与 Kibana Discover 完全一致的多字段短语匹配
	// 时间范围用 UTC 时间避免时区歧义
	from := time.Now().UTC().Add(-2 * 24 * time.Hour) // 最近2天，匹配 Kibana 的时间窗口
	to := time.Now().UTC()
	result, err := adapter.Search(atx, logprobe.SearchRequest{
		Query: "1017484319252605462", // 与 Kibana 中成功的查询完全一致
		TimeRange: logprobe.TimeRange{
			From: from,
			To:   to,
		},
		Limit:       50,  // 先拿10条看看
		FieldMaxLen: 100, // 长字段截断
		Fields:      []string{"@timestamp", "level", "message"},
		Format:      logprobe.FormatCompact,
	})
	if err != nil {
		t.Fatal("Search failed:", err)
	}
	atx.Info("result", logit.JSONDetailAbsField("result", result))
}

func Test_Stats(t *testing.T) {
	atx := appctx.New(
		appctx.WithLogger(logit.NewSimpleWithConfig(os.Stdout, &logit.Config{
			Level: logit.Debug,
		})),
	)

	adapter := testInit(atx)

	from := time.Now().UTC().Add(-2 * 24 * time.Hour) // 最近2天，匹配 Kibana 的时间窗口
	to := time.Now().UTC()
	sq := logprobe.SearchRequest{
		Query: "1017484319252605462", // 与 Kibana 中成功的查询完全一致
		TimeRange: logprobe.TimeRange{
			From: from,
			To:   to,
		},
		Limit:       50,  // 先拿10条看看
		FieldMaxLen: 100, // 长字段截断
		Fields:      []string{"@timestamp", "level", "message"},
		Format:      logprobe.FormatCompact,
	}

	stats, err := adapter.Stats(atx, logprobe.StatsRequest{
		Filter: &sq,
		Field:  "level",
		TopN:   0,
	})
	if err != nil {
		t.Fatal("Stats failed:", err)
	}
	atx.Info("stats", logit.JSONDetailAbsField("stats", stats))
}
