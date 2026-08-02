package request

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/izliang/bore/propagator/carrier"
	sre "github.com/izliang/bore/propagator/sre"
	"github.com/izliang/bore/request/tripper"
	"github.com/izliang/bore/web/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSREPropagator_InjectHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     sre.SREPropagatorData
		expected map[string]string
	}{
		{
			name: "single app with env",
			data: sre.SREPropagatorData{
				Env: "prod",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
				},
			},
			expected: map[string]string{
				"x-sre-env":           "prod",
				"brh-gray-risk":       "v1",
				"brh-subversion-risk": "20240520",
			},
		},
		{
			name: "multiple apps",
			data: sre.SREPropagatorData{
				Env: "test",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
					{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
					{ServerName: "user", BrhGrayVersion: "v3", BrhSubVersion: "20240522"},
				},
			},
			expected: map[string]string{
				"x-sre-env":             "test",
				"brh-gray-risk":         "v1",
				"brh-subversion-risk":   "20240520",
				"brh-gray-credit":       "v2",
				"brh-subversion-credit": "20240521",
				"brh-gray-user":         "v3",
				"brh-subversion-user":   "20240522",
			},
		},
		{
			name: "env only - no apps",
			data: sre.SREPropagatorData{
				Env:  "staging",
				Apps: nil,
			},
			expected: map[string]string{
				"x-sre-env": "staging",
			},
		},
		{
			name: "partial fields - some empty",
			data: sre.SREPropagatorData{
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1"},
					{ServerName: "credit", BrhSubVersion: "20240520"},
				},
			},
			expected: map[string]string{
				"brh-gray-risk":         "v1",
				"brh-subversion-credit": "20240520",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := sre.Set(context.Background(), tt.data)
			header := make(http.Header)

			p := sre.New()
			p.Inject(ctx, &carrier.HTTPHeader{Header: header})

			for key, expectedValue := range tt.expected {
				assert.Equal(t, expectedValue, header.Get(key), "header %s mismatch", key)
			}

			assert.Len(t, header, len(tt.expected), "unexpected number of headers")
		})
	}
}

func TestSREPropagator_ExtractHTTP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		expected sre.SREPropagatorData
	}{
		{
			name: "single app with env",
			headers: map[string]string{
				"x-sre-env":           "prod",
				"brh-gray-risk":       "v1",
				"brh-subversion-risk": "20240520",
			},
			expected: sre.SREPropagatorData{
				Env: "prod",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
				},
			},
		},
		{
			name: "multiple apps mixed case headers",
			headers: map[string]string{
				"X-Sre-Env":           "test",
				"Brh-Gray-Risk":       "v1",
				"Brh-Subversion-Risk": "20240520",
				"BRH-GRAY-CREDIT":     "v2",
				"brh-subversion-user": "20240522",
			},
			expected: sre.SREPropagatorData{
				Env: "test",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
					{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: ""},
					{ServerName: "user", BrhGrayVersion: "", BrhSubVersion: "20240522"},
				},
			},
		},
		{
			name:    "no sre headers - empty result",
			headers: map[string]string{"Content-Type": "application/json"},
			expected: sre.SREPropagatorData{
				Env:  "",
				Apps: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			header := make(http.Header)
			for k, v := range tt.headers {
				header.Set(k, v)
			}

			p := sre.New()
			ctx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: header})
			actual := sre.Get(ctx)

			require.NotNil(t, actual)
			assert.Equal(t, tt.expected.Env, actual.Env)

			actualApps := make(map[string]sre.SREPropagatorItem)
			for _, app := range actual.Apps {
				actualApps[app.ServerName] = app
			}

			for _, expectedApp := range tt.expected.Apps {
				actualApp, exists := actualApps[expectedApp.ServerName]
				require.True(t, exists, "missing server %s", expectedApp.ServerName)
				assert.Equal(t, expectedApp.BrhGrayVersion, actualApp.BrhGrayVersion, "server %s gray version mismatch", expectedApp.ServerName)
				assert.Equal(t, expectedApp.BrhSubVersion, actualApp.BrhSubVersion, "server %s sub version mismatch", expectedApp.ServerName)
			}
		})
	}
}

func TestSREPropagator_RoundTrip(t *testing.T) {
	t.Parallel()

	original := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
			{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
		},
	}

	ctx := sre.Set(context.Background(), original)
	header := make(http.Header)
	p := sre.New()
	p.Inject(ctx, &carrier.HTTPHeader{Header: header})

	newCtx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: header})
	extracted := sre.Get(newCtx)

	require.NotNil(t, extracted)
	assert.Equal(t, original.Env, extracted.Env)
	assert.Len(t, extracted.Apps, len(original.Apps))

	extractedMap := make(map[string]sre.SREPropagatorItem)
	for _, app := range extracted.Apps {
		extractedMap[app.ServerName] = app
	}

	for _, orig := range original.Apps {
		ext := extractedMap[orig.ServerName]
		assert.Equal(t, orig.BrhGrayVersion, ext.BrhGrayVersion)
		assert.Equal(t, orig.BrhSubVersion, ext.BrhSubVersion)
	}
}

func TestSREPropagator_WithExtraHeaders(t *testing.T) {
	t.Parallel()

	p := sre.New(
		sre.WithMap("x-request-id", "req-123"),
		sre.WithMap("x-user-id", "user-456"),
	)

	ctx := sre.Set(context.Background(), sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1"},
		},
	})

	header := make(http.Header)
	p.Inject(ctx, &carrier.HTTPHeader{Header: header})

	assert.Equal(t, "prod", header.Get("x-sre-env"))
	assert.Equal(t, "v1", header.Get("brh-gray-risk"))
	assert.Equal(t, "req-123", header.Get("x-request-id"))
	assert.Equal(t, "user-456", header.Get("x-user-id"))
}

func TestSREPropagator_HTTPExtraRoundTrip(t *testing.T) {
	t.Parallel()

	p := sre.New(
		sre.WithKey("x-request-id"),
		sre.WithMap("x-tenant-id", "default-tenant"),
	)

	incoming := make(http.Header)
	incoming.Set("x-sre-env", "prod")
	incoming.Set("brh-gray-risk", "v1")
	incoming.Set("x-request-id", "req-123")
	incoming.Set("x-tenant-id", "tenant-456")

	ctx := p.Extract(context.Background(), &carrier.HTTPHeader{Header: incoming})
	data := sre.Get(ctx)
	require.NotNil(t, data)
	assert.Equal(t, map[string]string{
		"x-request-id": "req-123",
		"x-tenant-id":  "tenant-456",
	}, data.Extra)

	outgoing := make(http.Header)
	p.Inject(ctx, &carrier.HTTPHeader{Header: outgoing})

	assert.Equal(t, "prod", outgoing.Get("x-sre-env"))
	assert.Equal(t, "v1", outgoing.Get("brh-gray-risk"))
	assert.Equal(t, "req-123", outgoing.Get("x-request-id"))
	assert.Equal(t, "tenant-456", outgoing.Get("x-tenant-id"))
}

func TestSREPropagator_InjectHTTPOverwritesExistingHeaders(t *testing.T) {
	t.Parallel()

	ctx := sre.Set(context.Background(), sre.SREPropagatorData{Env: "prod"})
	header := make(http.Header)
	header.Add("x-sre-env", "old")
	header.Add("x-sre-env", "older")

	p := sre.New()
	p.Inject(ctx, &carrier.HTTPHeader{Header: header})

	assert.Equal(t, "prod", header.Get("x-sre-env"))
	assert.Equal(t, []string{"prod"}, header.Values("x-sre-env"))
}

func TestPropagatorEndToEnd(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	sreProp := sre.New()

	// 创建自动透传的 HTTP Client - 使用 PropagatorRoundTripper
	// 这样发起 HTTP 请求时会自动从 context 提取数据注入到 Header，无需手动调用
	httpTransport := tripper.Build(
		tripper.Propagation(sreProp),
	)
	autoPropagateClient := &http.Client{Transport: httpTransport}

	// 服务 B - 使用中间件自动提取 header 并验证
	serverBRouter := gin.New()
	serverBRouter.Use(middleware.PropagatorMiddleware(sreProp))
	serverBRouter.GET("/", func(c *gin.Context) {
		data := sre.Get(c.Request.Context())
		require.NotNil(t, data)
		assert.Equal(t, "prod", data.Env)
		require.Len(t, data.Apps, 1)
		assert.Equal(t, "risk", data.Apps[0].ServerName)
		assert.Equal(t, "v1", data.Apps[0].BrhGrayVersion)
		assert.Equal(t, "20240520", data.Apps[0].BrhSubVersion)
		c.Status(http.StatusOK)
	})
	serverB := httptest.NewServer(serverBRouter)
	defer serverB.Close()

	// 服务 A - 使用中间件提取 header，然后调用服务 B（自动透传）
	serverARouter := gin.New()
	serverARouter.Use(middleware.PropagatorMiddleware(sreProp))
	serverARouter.GET("/", func(c *gin.Context) {
		// 从请求 context 中获取透传数据，验证中间件已正确提取
		data := sre.Get(c.Request.Context())
		require.NotNil(t, data)

		// ✨ 关键：使用自动透传的 Client，只需传入 context，无需手动 InjectHTTP
		// PropagatorRoundTripper 会自动从 context 中提取数据并注入到 Header
		req, _ := http.NewRequestWithContext(c.Request.Context(), "GET", serverB.URL, nil)
		// sreProp.Inject(ctx, &carrier.HTTPHeader{Header: req.Header}) ← 不需要手动调用了！
		resp, err := autoPropagateClient.Do(req)

		require.NoError(t, err)
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		c.Status(http.StatusOK)
	})
	serverA := httptest.NewServer(serverARouter)
	defer serverA.Close()

	// 发起请求
	originalData := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
		},
	}
	ctx := sre.Set(context.Background(), originalData)

	req, _ := http.NewRequestWithContext(ctx, "GET", serverA.URL, nil)
	sreProp.Inject(ctx, &carrier.HTTPHeader{Header: req.Header})

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
