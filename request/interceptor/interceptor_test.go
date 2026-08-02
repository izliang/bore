package interceptor

import (
	"bytes"
	"context"
	"testing"

	"github.com/izliang/bore/logit"
	sre "github.com/izliang/bore/propagator/sre"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestLogitUnaryClientInterceptor_LogsInjectedMetadataWhenOrderedAfterPropagation(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logit.NewSimple(&buf)
	sreProp := sre.New()
	chain := chainUnaryClientInterceptors(
		UnaryClientInterceptor(sreProp),
		LogitUnaryClientInterceptor(logger),
	)

	ctx := sre.Set(context.Background(), sre.SREPropagatorData{
		Env: "test",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
		},
	})
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "req-1")

	err := chain(ctx, "/test.Service/Method", nil, nil, nil, noopUnaryInvoker)
	require.NoError(t, err)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "grpc unary client metadata")
	assert.Contains(t, logOutput, "method[/test.Service/Method]")
	assert.Contains(t, logOutput, `"x-request-id":["req-1"]`)
	assert.Contains(t, logOutput, `"x-sre-env":["test"]`)
	assert.Contains(t, logOutput, `"brh-gray-risk":["v1"]`)
	assert.Contains(t, logOutput, `"brh-subversion-risk":["20240520"]`)
}

func TestLogitUnaryClientInterceptor_DoesNotLogLaterInjectedMetadata(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logit.NewSimple(&buf)
	sreProp := sre.New()
	chain := chainUnaryClientInterceptors(
		LogitUnaryClientInterceptor(logger),
		UnaryClientInterceptor(sreProp),
	)

	ctx := sre.Set(context.Background(), sre.SREPropagatorData{
		Env: "test",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
		},
	})
	ctx = metadata.AppendToOutgoingContext(ctx, "x-request-id", "req-1")

	err := chain(ctx, "/test.Service/Method", nil, nil, nil, noopUnaryInvoker)
	require.NoError(t, err)

	logOutput := buf.String()
	assert.Contains(t, logOutput, `"x-request-id":["req-1"]`)
	assert.NotContains(t, logOutput, "x-sre-env")
	assert.NotContains(t, logOutput, "brh-gray-risk")
	assert.NotContains(t, logOutput, "brh-subversion-risk")
}

func chainUnaryClientInterceptors(interceptors ...grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		chainedInvoker := invoker
		for i := len(interceptors) - 1; i >= 0; i-- {
			currentInterceptor := interceptors[i]
			nextInvoker := chainedInvoker
			chainedInvoker = func(interceptor grpc.UnaryClientInterceptor, next grpc.UnaryInvoker) grpc.UnaryInvoker {
				return func(currentCtx context.Context, currentMethod string, currentReq, currentReply any, currentCC *grpc.ClientConn, currentOpts ...grpc.CallOption) error {
					return interceptor(currentCtx, currentMethod, currentReq, currentReply, currentCC, next, currentOpts...)
				}
			}(currentInterceptor, nextInvoker)
		}
		return chainedInvoker(ctx, method, req, reply, cc, opts...)
	}
}

func noopUnaryInvoker(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
	return nil
}

func TestLogitUnaryServerInterceptor_LogsIncomingMetadata(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logit.NewSimple(&buf)
	interceptor := LogitUnaryServerInterceptor(logger)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"x-request-id", "req-1",
		"x-sre-env", "test",
	))

	called := false
	resp, err := interceptor(ctx, "request", &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, func(ctx context.Context, req any) (any, error) {
		called = true
		assert.Equal(t, "request", req)
		return "response", nil
	})
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, "response", resp)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "grpc unary server metadata")
	assert.Contains(t, logOutput, "method[/test.Service/Method]")
	assert.Contains(t, logOutput, `"x-request-id":["req-1"]`)
	assert.Contains(t, logOutput, `"x-sre-env":["test"]`)
}

func TestLogitUnaryServerInterceptor_LogsEmptyMetadata(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logit.NewSimple(&buf)
	interceptor := LogitUnaryServerInterceptor(logger)

	_, err := interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/test.Service/Method"}, noopUnaryHandler)
	require.NoError(t, err)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "grpc unary server metadata")
	assert.Contains(t, logOutput, "method[/test.Service/Method]")
}

func noopUnaryHandler(ctx context.Context, req any) (any, error) {
	return nil, nil
}

func TestLogitUnaryClientInterceptor_LogsEmptyMetadata(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := logit.NewSimple(&buf)
	interceptor := LogitUnaryClientInterceptor(logger)

	err := interceptor(context.Background(), "/test.Service/Method", nil, nil, nil, noopUnaryInvoker)
	require.NoError(t, err)

	logOutput := buf.String()
	assert.Contains(t, logOutput, "grpc unary client metadata")
	assert.Contains(t, logOutput, "method[/test.Service/Method]")
}
