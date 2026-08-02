package request

import (
	"context"
	"testing"

	"github.com/izliang/bore/propagator/carrier"
	sre "github.com/izliang/bore/propagator/sre"
	"github.com/izliang/bore/request/interceptor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// ==================== gRPC 拦截器测试辅助函数 ====================

// mockInvoker 模拟的 gRPC 调用器，用于测试客户端拦截器
func mockInvoker(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
	// 从 context 中提取 metadata，验证注入成功
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return nil
	}

	// 将 metadata 存入 reply 以便测试验证
	if r, ok := reply.(*mockResponse); ok {
		r.md = md
	}
	return nil
}

// mockResponse 模拟的 gRPC 响应，用于存储 metadata
type mockResponse struct {
	md metadata.MD
}

// mockHandler 模拟的 gRPC 服务端 handler，用于测试服务端拦截器
func mockHandler(ctx context.Context, req interface{}) (interface{}, error) {
	// 从 context 中提取透传数据
	data := sre.Get(ctx)
	return &mockHandlerResult{data: data}, nil
}

// mockHandlerResult 模拟的 handler 返回结果
type mockHandlerResult struct {
	data *sre.SREPropagatorData
}

// ==================== 单元测试：InjectGRPC 和 ExtractGRPC ====================

func TestSREPropagator_InjectGRPC(t *testing.T) {
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
				},
			},
			expected: map[string]string{
				"x-sre-env":             "test",
				"brh-gray-risk":         "v1",
				"brh-subversion-risk":   "20240520",
				"brh-gray-credit":       "v2",
				"brh-subversion-credit": "20240521",
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
			name: "with extra headers",
			data: sre.SREPropagatorData{
				Env: "prod",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1"},
				},
			},
			expected: map[string]string{
				"x-sre-env":     "prod",
				"brh-gray-risk": "v1",
				"x-request-id":  "req-123",
				"x-user-id":     "user-456",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := sre.Set(context.Background(), tt.data)
			md := metadata.New(nil)

			p := sre.New()
			if tt.name == "with extra headers" {
				p = sre.New(
					sre.WithMap("x-request-id", "req-123"),
					sre.WithMap("x-user-id", "user-456"),
				)
			}

			p.Inject(ctx, &carrier.GRPCMetadata{MD: md})

			for key, expectedValue := range tt.expected {
				values := md.Get(key)
				require.Len(t, values, 1, "header %s should exist", key)
				assert.Equal(t, expectedValue, values[0], "header %s mismatch", key)
			}
		})
	}
}

func TestSREPropagator_ExtractGRPC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		md       map[string]string
		expected sre.SREPropagatorData
	}{
		{
			name: "single app with env",
			md: map[string]string{
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
			name: "multiple apps",
			md: map[string]string{
				"x-sre-env":             "test",
				"brh-gray-risk":         "v1",
				"brh-subversion-risk":   "20240520",
				"brh-gray-credit":       "v2",
				"brh-subversion-credit": "20240521",
			},
			expected: sre.SREPropagatorData{
				Env: "test",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
					{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
				},
			},
		},
		{
			name: "mixed case metadata keys (gRPC is lowercase)",
			md: map[string]string{
				"X-Sre-Env":           "prod",
				"BRH-GRAY-RISK":       "v1",
				"Brh-Subversion-Risk": "20240520",
			},
			expected: sre.SREPropagatorData{
				Env: "prod",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
				},
			},
		},
		{
			name:     "no sre metadata - empty result",
			md:       map[string]string{"content-type": "application/grpc"},
			expected: sre.SREPropagatorData{Env: "", Apps: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.New(nil)
			for k, v := range tt.md {
				md.Set(k, v)
			}

			p := sre.New()
			ctx := metadata.NewIncomingContext(context.Background(), md)
			newCtx := p.Extract(ctx, &carrier.GRPCMetadata{MD: md})

			actual := sre.Get(newCtx)
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

func TestSREPropagator_GRPCRoundTrip(t *testing.T) {
	t.Parallel()

	original := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
			{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
		},
	}

	// 注入
	ctx := sre.Set(context.Background(), original)
	md := metadata.New(nil)
	p := sre.New()
	p.Inject(ctx, &carrier.GRPCMetadata{MD: md})

	// 提取（模拟 gRPC 服务器端）
	incomingCtx := metadata.NewIncomingContext(context.Background(), md)
	newCtx := p.Extract(incomingCtx, &carrier.GRPCMetadata{MD: md})
	extracted := sre.Get(newCtx)

	// 验证
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

func TestSREPropagator_GRPCExtraRoundTrip(t *testing.T) {
	t.Parallel()

	p := sre.New(
		sre.WithKey("x-request-id"),
		sre.WithMap("x-tenant-id", "default-tenant"),
	)
	md := metadata.New(map[string]string{
		"x-sre-env":    "prod",
		"x-request-id": "req-123",
		"x-tenant-id":  "tenant-456",
	})

	ctx := p.Extract(metadata.NewIncomingContext(context.Background(), md), &carrier.GRPCMetadata{MD: md})
	data := sre.Get(ctx)
	require.NotNil(t, data)
	assert.Equal(t, map[string]string{
		"x-request-id": "req-123",
		"x-tenant-id":  "tenant-456",
	}, data.Extra)

	outgoing := metadata.New(nil)
	p.Inject(ctx, &carrier.GRPCMetadata{MD: outgoing})

	assert.Equal(t, []string{"prod"}, outgoing.Get("x-sre-env"))
	assert.Equal(t, []string{"req-123"}, outgoing.Get("x-request-id"))
	assert.Equal(t, []string{"tenant-456"}, outgoing.Get("x-tenant-id"))
}

// ==================== 单元测试：UnaryClientInterceptor ====================

func TestGRPCUnaryClientInterceptor_Inject(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	interceptor := interceptor.UnaryClientInterceptor(sreProp)

	tests := []struct {
		name     string
		data     sre.SREPropagatorData
		expected map[string]string
	}{
		{
			name: "inject single app data",
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
			name: "inject multiple apps data",
			data: sre.SREPropagatorData{
				Env: "test",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
					{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
				},
			},
			expected: map[string]string{
				"x-sre-env":             "test",
				"brh-gray-risk":         "v1",
				"brh-subversion-risk":   "20240520",
				"brh-gray-credit":       "v2",
				"brh-subversion-credit": "20240521",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 将数据放入 context
			ctx := sre.Set(context.Background(), tt.data)

			// 调用拦截器
			reply := &mockResponse{}
			err := interceptor(ctx, "/test.Service/Method", nil, reply, nil, mockInvoker)

			require.NoError(t, err)

			// 验证 metadata 中的内容
			for key, expectedValue := range tt.expected {
				values := reply.md.Get(key)
				require.Len(t, values, 1, "header %s should exist", key)
				assert.Equal(t, expectedValue, values[0], "header %s mismatch", key)
			}
		})
	}
}

// ==================== 单元测试：UnaryServerInterceptor ====================

func TestGRPCUnaryServerInterceptor_Extract(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	interceptor := interceptor.UnaryServerInterceptor(sreProp)

	tests := []struct {
		name     string
		md       map[string]string
		expected sre.SREPropagatorData
	}{
		{
			name: "extract single app data",
			md: map[string]string{
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
			name: "extract multiple apps data",
			md: map[string]string{
				"x-sre-env":             "test",
				"brh-gray-risk":         "v1",
				"brh-subversion-risk":   "20240520",
				"brh-gray-credit":       "v2",
				"brh-subversion-credit": "20240521",
			},
			expected: sre.SREPropagatorData{
				Env: "test",
				Apps: []sre.SREPropagatorItem{
					{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
					{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建带 metadata 的 incoming context
			md := metadata.New(nil)
			for k, v := range tt.md {
				md.Set(k, v)
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			// 调用拦截器
			info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
			result, err := interceptor(ctx, nil, info, mockHandler)

			require.NoError(t, err)
			actualResult := result.(*mockHandlerResult)

			// 验证提取的数据
			require.NotNil(t, actualResult.data)
			assert.Equal(t, tt.expected.Env, actualResult.data.Env)

			actualApps := make(map[string]sre.SREPropagatorItem)
			for _, app := range actualResult.data.Apps {
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

// ==================== 端到端测试：一元 gRPC 完整透传链路 ====================

func TestGRPCPropagatorEndToEnd_Unary(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	clientInterceptor := interceptor.UnaryClientInterceptor(sreProp)
	serverInterceptor := interceptor.UnaryServerInterceptor(sreProp)

	// 准备测试数据
	testData := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
		},
	}
	ctx := sre.Set(context.Background(), testData)

	// 模拟完整链路：客户端拦截器 -> 网络传输 -> 服务端拦截器 -> handler

	// 步骤 1: 客户端拦截器注入
	var capturedMd metadata.MD
	captureInvoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			capturedMd = md.Copy()
		}
		return nil
	}

	err := clientInterceptor(ctx, "/test.Service/Method", nil, nil, nil, captureInvoker)
	require.NoError(t, err)
	require.NotNil(t, capturedMd)

	// 验证注入正确
	assert.Equal(t, "prod", capturedMd.Get("x-sre-env")[0])
	assert.Equal(t, "v1", capturedMd.Get("brh-gray-risk")[0])
	assert.Equal(t, "20240520", capturedMd.Get("brh-subversion-risk")[0])

	// 步骤 2: 模拟网络传输后，服务端收到 metadata
	incomingCtx := metadata.NewIncomingContext(context.Background(), capturedMd)

	// 步骤 3: 服务端拦截器提取
	var extractedData *sre.SREPropagatorData
	verifyHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		extractedData = sre.Get(ctx)
		return nil, nil
	}

	info := &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}
	_, err = serverInterceptor(incomingCtx, nil, info, verifyHandler)
	require.NoError(t, err)

	// 验证提取正确
	require.NotNil(t, extractedData)
	assert.Equal(t, "prod", extractedData.Env)
	require.Len(t, extractedData.Apps, 1)
	assert.Equal(t, "risk", extractedData.Apps[0].ServerName)
	assert.Equal(t, "v1", extractedData.Apps[0].BrhGrayVersion)
	assert.Equal(t, "20240520", extractedData.Apps[0].BrhSubVersion)

	t.Log("✅ gRPC 一元调用透传链路验证成功")
}

// ==================== 端到端测试：多跳透传 ====================

func TestGRPCPropagatorEndToEnd_MultiHop(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	clientInterceptor := interceptor.UnaryClientInterceptor(sreProp)
	serverInterceptor := interceptor.UnaryServerInterceptor(sreProp)

	// 准备测试数据
	testData := sre.SREPropagatorData{
		Env: "staging",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
			{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
		},
	}

	// ========== 第 1 跳：客户端 -> 服务 A ==========
	ctx := sre.Set(context.Background(), testData)

	// 服务 A 的客户端拦截器注入
	var mdAtA metadata.MD
	captureInvokerA := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			mdAtA = md.Copy()
		}
		return nil
	}

	err := clientInterceptor(ctx, "/test.Service/MethodA", nil, nil, nil, captureInvokerA)
	require.NoError(t, err)

	// ========== 第 2 跳：服务 A -> 服务 B ==========

	// 服务 A 的服务端拦截器提取
	incomingAtA := metadata.NewIncomingContext(context.Background(), mdAtA)

	// 服务 A 调用服务 B 的客户端拦截器注入
	var mdAtB metadata.MD
	captureInvokerB := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			mdAtB = md.Copy()
		}
		return nil
	}

	// 模拟服务 A 的 handler 调用服务 B
	forwardHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		// handler 中发起新的 gRPC 调用，会自动携带 context 中的透传数据
		return nil, clientInterceptor(ctx, "/test.Service/MethodB", nil, nil, nil, captureInvokerB)
	}

	infoA := &grpc.UnaryServerInfo{FullMethod: "/test.Service/MethodA"}
	_, err = serverInterceptor(incomingAtA, nil, infoA, forwardHandler)
	require.NoError(t, err)

	// ========== 第 3 跳：服务 B 提取验证 ==========
	incomingAtB := metadata.NewIncomingContext(context.Background(), mdAtB)

	var finalData *sre.SREPropagatorData
	finalHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		finalData = sre.Get(ctx)
		return nil, nil
	}

	infoB := &grpc.UnaryServerInfo{FullMethod: "/test.Service/MethodB"}
	_, err = serverInterceptor(incomingAtB, nil, infoB, finalHandler)
	require.NoError(t, err)

	// ========== 验证多跳透传成功 ==========
	require.NotNil(t, finalData)
	assert.Equal(t, "staging", finalData.Env)
	require.Len(t, finalData.Apps, 2)

	appMap := make(map[string]sre.SREPropagatorItem)
	for _, app := range finalData.Apps {
		appMap[app.ServerName] = app
	}

	assert.Equal(t, "v1", appMap["risk"].BrhGrayVersion)
	assert.Equal(t, "20240520", appMap["risk"].BrhSubVersion)
	assert.Equal(t, "v2", appMap["credit"].BrhGrayVersion)
	assert.Equal(t, "20240521", appMap["credit"].BrhSubVersion)

	t.Log("✅ gRPC 多跳透传链路验证成功")
}

// ==================== 单元测试：StreamClientInterceptor ====================

func TestGRPCStreamClientInterceptor_Inject(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	interceptor := interceptor.StreamClientInterceptor(sreProp)

	// 准备测试数据
	testData := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
		},
	}
	ctx := sre.Set(context.Background(), testData)

	// 模拟 streamer
	var capturedMd metadata.MD
	mockStreamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		md, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			capturedMd = md.Copy()
		}
		return &mockClientStream{ctx: ctx}, nil
	}

	// 调用拦截器
	desc := &grpc.StreamDesc{StreamName: "TestStream"}
	stream, err := interceptor(ctx, desc, nil, "/test.Service/Stream", mockStreamer)

	require.NoError(t, err)
	require.NotNil(t, stream)

	// 验证 metadata 注入正确
	assert.Equal(t, "prod", capturedMd.Get("x-sre-env")[0])
	assert.Equal(t, "v1", capturedMd.Get("brh-gray-risk")[0])
	assert.Equal(t, "20240520", capturedMd.Get("brh-subversion-risk")[0])

	t.Log("✅ gRPC 流式客户端拦截器注入验证成功")
}

// ==================== 单元测试：StreamServerInterceptor ====================

func TestGRPCStreamServerInterceptor_Extract(t *testing.T) {
	t.Parallel()

	sreProp := sre.New()
	interceptor := interceptor.StreamServerInterceptor(sreProp)

	// 准备 metadata
	md := metadata.New(map[string]string{
		"x-sre-env":           "prod",
		"brh-gray-risk":       "v1",
		"brh-subversion-risk": "20240520",
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	// 模拟 server stream
	serverStream := &mockServerStream{ctx: ctx}

	// 模拟 handler - 从 stream context 中提取数据
	var extractedData *sre.SREPropagatorData
	mockHandler := func(srv interface{}, stream grpc.ServerStream) error {
		extractedData = sre.Get(stream.Context())
		return nil
	}

	// 调用拦截器
	info := &grpc.StreamServerInfo{FullMethod: "/test.Service/Stream"}
	err := interceptor(nil, serverStream, info, mockHandler)

	require.NoError(t, err)
	require.NotNil(t, extractedData)
	assert.Equal(t, "prod", extractedData.Env)
	require.Len(t, extractedData.Apps, 1)
	assert.Equal(t, "risk", extractedData.Apps[0].ServerName)

	t.Log("✅ gRPC 流式服务端拦截器提取验证成功")
}

// mockClientStream 模拟的 ClientStream
type mockClientStream struct {
	ctx context.Context
	grpc.ClientStream
}

func (m *mockClientStream) Context() context.Context {
	return m.ctx
}

// mockServerStream 模拟的 ServerStream
type mockServerStream struct {
	ctx context.Context
	grpc.ServerStream
}

func (m *mockServerStream) Context() context.Context {
	return m.ctx
}
