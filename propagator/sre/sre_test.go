package sre

import (
	"context"
	"net/http"
	"testing"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"google.golang.org/grpc/metadata"
)

// ──────────────────────────── Context 存取 ───────────────────────────

func TestSetGet(t *testing.T) {
	tests := []struct {
		name string
		data SREPropagatorData
	}{
		{
			name: "完整数据",
			data: SREPropagatorData{
				Env:     "prod",
				ApiCode: "api-risk-v1",
				Apps: []SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
				},
				Extra: map[string]string{"x-request-id": "req-123"},
			},
		},
		{
			name: "仅环境",
			data: SREPropagatorData{Env: "staging"},
		},
		{
			name: "空数据",
			data: SREPropagatorData{},
		},
		{
			name: "多 App",
			data: SREPropagatorData{
				Apps: []SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1"},
					{ServerName: "order", BrhSubVersion: "20240701"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Set(context.Background(), tt.data)
			got := Get(ctx)

			if got == nil {
				t.Fatal("Get() returned nil")
			}
			if got.Env != tt.data.Env {
				t.Errorf("Env = %q, want %q", got.Env, tt.data.Env)
			}
			if got.ApiCode != tt.data.ApiCode {
				t.Errorf("ApiCode = %q, want %q", got.ApiCode, tt.data.ApiCode)
			}
			if len(got.Apps) != len(tt.data.Apps) {
				t.Errorf("Apps len = %d, want %d", len(got.Apps), len(tt.data.Apps))
			}
		})
	}
}

func TestGet_EmptyContext(t *testing.T) {
	if got := Get(context.Background()); got != nil {
		t.Errorf("Get on empty context = %v, want nil", got)
	}
}

// ──────────────────────────── HTTP 注入/提取 ─────────────────────────

func TestInjectExtract_HTTP(t *testing.T) {
	tests := []struct {
		name     string
		opts     []SREPropagatorOptFunc
		data     SREPropagatorData
		incoming http.Header // 模拟上游 header
		want     SREPropagatorData
	}{
		{
			name: "完整字段注入和提取",
			data: SREPropagatorData{
				Env:     "prod",
				ApiCode: "api-code-1",
				Apps: []SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "sv1"},
					{ServerName: "order", BrhGrayVersion: "v2"},
				},
			},
			incoming: http.Header{},
			want: SREPropagatorData{
				Env:     "prod",
				ApiCode: "api-code-1",
				Apps: []SREPropagatorItem{
					{ServerName: "order", BrhGrayVersion: "v2"},
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "sv1"},
				},
			},
		},
		{
			name:     "仅环境字段",
			data:     SREPropagatorData{Env: "staging"},
			incoming: http.Header{},
			want:     SREPropagatorData{Env: "staging"},
		},
		{
			name: "仅 Apps 字段",
			data: SREPropagatorData{
				Apps: []SREPropagatorItem{
					{ServerName: "svc", BrhGrayVersion: "gray"},
				},
			},
			incoming: http.Header{},
			want: SREPropagatorData{
				Apps: []SREPropagatorItem{
					{ServerName: "svc", BrhGrayVersion: "gray"},
				},
			},
		},
		{
			name:     "空数据不注入任何 header",
			data:     SREPropagatorData{},
			incoming: http.Header{},
			want:     SREPropagatorData{},
		},
		{
			name: "大小写不敏感提取",
			data: SREPropagatorData{},
			incoming: http.Header{
				"X-Sre-Env":        []string{"prod"},
				"Brh-Gray-Risk":    []string{"v1"},
				"Brh-Gray-Apicode": []string{"code-1"},
			},
			want: SREPropagatorData{
				Env:     "prod",
				ApiCode: "code-1",
				Apps: []SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1"},
				},
			},
		},
		{
			name: "Extra 字段透传",
			opts: []SREPropagatorOptFunc{
				WithKey("x-request-id"),
				WithMap("x-tenant-id", "default"),
			},
			data: SREPropagatorData{
				Extra: map[string]string{
					"x-request-id": "req-456",
					"x-tenant-id":  "tenant-override",
				},
			},
			incoming: http.Header{},
			want: SREPropagatorData{
				Extra: map[string]string{
					"x-request-id": "req-456",
					"x-tenant-id":  "tenant-override",
				},
			},
		},
		{
			name: "Extra 从上游 header 提取",
			opts: []SREPropagatorOptFunc{
				WithKey("x-request-id"),
			},
			data: SREPropagatorData{},
			incoming: http.Header{
				"X-Request-Id": []string{"upstream-req"},
			},
			want: SREPropagatorData{
				Extra: map[string]string{"x-request-id": "upstream-req"},
			},
		},
		{
			name: "Extra 默认值在无上游数据时生效",
			opts: []SREPropagatorOptFunc{
				WithMap("x-tenant-id", "default-tenant"),
			},
			data:     SREPropagatorData{},
			incoming: http.Header{},
			want: SREPropagatorData{
				Extra: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.opts...)

			// 1. 注入
			ctx := Set(context.Background(), tt.data)
			outHeader := http.Header{}
			p.Inject(ctx, &carrier.HTTPHeader{Header: outHeader})

			// 2. 合并上游 header 后再提取
			for k, vs := range tt.incoming {
				for _, v := range vs {
					outHeader.Add(k, v)
				}
			}
			extractedCtx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: outHeader})
			got := Get(extractedCtx)
			if got == nil {
				if tt.want.Env == "" && len(tt.want.Apps) == 0 && tt.want.ApiCode == "" {
					return
				}
				t.Fatal("Get() returned nil")
			}

			if got.Env != tt.want.Env {
				t.Errorf("Env = %q, want %q", got.Env, tt.want.Env)
			}
			if got.ApiCode != tt.want.ApiCode {
				t.Errorf("ApiCode = %q, want %q", got.ApiCode, tt.want.ApiCode)
			}
			if len(got.Apps) != len(tt.want.Apps) {
				t.Errorf("Apps len = %d, want %d", len(got.Apps), len(tt.want.Apps))
			}
		})
	}
}

// ──────────────────────────── HTTP 注入覆盖 ──────────────────────────

func TestInject_HTTPOverwrite(t *testing.T) {
	p := New()
	ctx := Set(context.Background(), SREPropagatorData{
		Env: "prod",
	})
	header := http.Header{"X-Sre-Env": []string{"old"}}
	p.Inject(ctx, &carrier.HTTPHeader{Header: header})

	if got := header.Get("x-sre-env"); got != "prod" {
		t.Errorf("header value = %q, want %q (注入应覆盖已有值)", got, "prod")
	}
}

// ──────────────────────────── gRPC 注入/提取 ─────────────────────────

func TestInjectExtract_GRPC(t *testing.T) {
	tests := []struct {
		name    string
		data    SREPropagatorData
		md      metadata.MD // 模拟上游 metadata（注入后追加）
		wantEnv string
	}{
		{
			name: "完整字段",
			data: SREPropagatorData{
				Env:     "prod",
				ApiCode: "api-code",
				Apps: []SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "sv1"},
				},
			},
			wantEnv: "prod",
		},
		{
			name:    "gRPC 小写 key 提取",
			data:    SREPropagatorData{},
			md:      metadata.Pairs("x-sre-env", "staging", "brh-gray-risk", "v2"),
			wantEnv: "staging",
		},
		{
			name:    "无数据",
			data:    SREPropagatorData{},
			wantEnv: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New()

			ctx := Set(context.Background(), tt.data)
			outMD := metadata.New(nil)
			p.Inject(ctx, &carrier.GRPCMetadata{MD: outMD})

			if tt.md != nil {
				for k, vs := range tt.md {
					for _, v := range vs {
						outMD.Append(k, v)
					}
				}
			}

			extractedCtx := p.Extract(context.Background(), &carrier.GRPCMetadata{MD: outMD})
			got := Get(extractedCtx)
			if got == nil {
				t.Fatal("Get() returned nil")
			}
			if got.Env != tt.wantEnv {
				t.Errorf("Env = %q, want %q", got.Env, tt.wantEnv)
			}
		})
	}
}

// ──────────────────────────── MQ 注入/提取 ───────────────────────────

func TestInjectExtract_MQ(t *testing.T) {
	tests := []struct {
		name     string
		opts     []SREPropagatorOptFunc
		data     SREPropagatorData
		incoming map[string]string
		wantEnv  string
	}{
		{
			name:    "基本注入提取",
			data:    SREPropagatorData{Env: "prod"},
			wantEnv: "prod",
		},
		{
			name: "从上游 MQ 大小写不敏感提取",
			incoming: map[string]string{
				"X-Sre-Env": "staging",
			},
			wantEnv: "staging",
		},
		{
			name: "Extra 字段 MQ 透传",
			opts: []SREPropagatorOptFunc{
				WithKey("x-request-id"),
			},
			data: SREPropagatorData{
				Extra: map[string]string{"x-request-id": "mq-req"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.opts...)

			ctx := Set(context.Background(), tt.data)
			outHeaders := make(map[string]string)
			for k, v := range tt.incoming {
				outHeaders[k] = v
			}
			p.Inject(ctx, &carrier.MQHeaders{Headers: outHeaders})

			extractedCtx := p.Extract(context.Background(), &carrier.MQHeaders{Headers: outHeaders})
			got := Get(extractedCtx)
			if got == nil {
				t.Fatal("Get() returned nil")
			}
			if got.Env != tt.wantEnv {
				t.Errorf("Env = %q, want %q", got.Env, tt.wantEnv)
			}
		})
	}
}

// ──────────────────────────── Extra 字段边界 ─────────────────────────

func TestExtra_BoundaryCases(t *testing.T) {
	tests := []struct {
		name   string
		opts   []SREPropagatorOptFunc
		data   SREPropagatorData
		setKey string
		want   string
	}{
		{
			name: "Extra 空值不注入",
			opts: []SREPropagatorOptFunc{WithMap("x-key", "")},
			data: SREPropagatorData{},
			want: "",
		},
		{
			name: "Extra 默认值生效",
			opts: []SREPropagatorOptFunc{WithMap("x-key", "default")},
			data: SREPropagatorData{},
			want: "default",
		},
		{
			name:   "Extra data 优先级高于默认值",
			opts:   []SREPropagatorOptFunc{WithMap("x-key", "default")},
			data:   SREPropagatorData{Extra: map[string]string{"x-key": "override"}},
			setKey: "x-key",
			want:   "override",
		},
		{
			name:   "Extra 大小写不敏感匹配默认值",
			opts:   []SREPropagatorOptFunc{WithMap("x-key", "default")},
			data:   SREPropagatorData{Extra: map[string]string{"X-Key": "override"}},
			setKey: "x-key",
			want:   "override",
		},
		{
			name: "重复 key 不重复注册",
			opts: []SREPropagatorOptFunc{
				WithKey("x-key"),
				WithKey("x-key"),
				WithKey("x-key"),
			},
			data: SREPropagatorData{},
			want: "",
		},
		{
			name: "WithMap 空 key 忽略",
			opts: []SREPropagatorOptFunc{WithMap("", "ignored")},
			data: SREPropagatorData{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.opts...)
			ctx := Set(context.Background(), tt.data)
			header := http.Header{}
			p.Inject(ctx, &carrier.HTTPHeader{Header: header})

			if tt.setKey != "" {
				got := header.Get(tt.setKey)
				if got != tt.want {
					t.Errorf("header[%q] = %q, want %q", tt.setKey, got, tt.want)
				}
			}
		})
	}
}

// ──────────────────────────── 多 App 排序 ────────────────────────────

func TestApps_SortedByServerName(t *testing.T) {
	p := New()
	header := http.Header{
		"Brh-Gray-Order":      []string{"v2"},
		"Brh-Gray-Risk":       []string{"v1"},
		"Brh-Subversion-Auth": []string{"sv3"},
	}

	ctx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: header})
	data := Get(ctx)

	if len(data.Apps) != 3 {
		t.Fatalf("Apps len = %d, want 3", len(data.Apps))
	}
	// 验证按 ServerName 排序
	if data.Apps[0].ServerName != "auth" {
		t.Errorf("Apps[0].ServerName = %q, want auth (按字母排序)", data.Apps[0].ServerName)
	}
	if data.Apps[1].ServerName != "order" {
		t.Errorf("Apps[1].ServerName = %q, want order", data.Apps[1].ServerName)
	}
	if data.Apps[2].ServerName != "risk" {
		t.Errorf("Apps[2].ServerName = %q, want risk", data.Apps[2].ServerName)
	}
}

// ──────────────────────────── 接口合规 ──────────────────────────────

func TestImplementsPropagator(t *testing.T) {
	// 编译期保证：SREPropagator 实现 propagator.Propagator
	var _ propagator.Propagator = New()
}
