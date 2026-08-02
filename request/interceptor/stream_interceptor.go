package interceptor

import (
	"context"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// wrappedClientStream 包装后的 ClientStream，用于拦截器中传递 context
type wrappedClientStream struct {
	grpc.ClientStream
	ctx context.Context
}

// Context 返回包装后的 context
func (w *wrappedClientStream) Context() context.Context {
	return w.ctx
}

// StreamClientInterceptor gRPC 流式客户端拦截器
// 用于在发送请求前将 context 中的透传数据注入到 gRPC metadata
func StreamClientInterceptor(props ...propagator.Propagator) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		// 创建新的 metadata 或使用现有的
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		} else {
			md = md.Copy() // 避免修改原始 metadata
		}

		// 将所有 Propagator 的数据注入到 metadata
		carr := &carrier.GRPCMetadata{MD: md}
		for _, prop := range props {
			prop.Inject(ctx, carr)
		}

		// 使用新的 metadata 创建 context
		newCtx := metadata.NewOutgoingContext(ctx, md)

		// 创建流
		clientStream, err := streamer(newCtx, desc, cc, method, opts...)
		if err != nil {
			return nil, err
		}

		// 包装流，使用新的 context
		return &wrappedClientStream{
			ClientStream: clientStream,
			ctx:          newCtx,
		}, nil
	}
}

// wrappedServerStream 包装后的 ServerStream，用于拦截器中传递 context
type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

// Context 返回包装后的 context
func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// StreamServerInterceptor gRPC 流式服务端拦截器
// 用于从接收的 gRPC metadata 中提取透传数据，放入 context
func StreamServerInterceptor(props ...propagator.Propagator) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()

		// 从 incoming context 中获取 metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			// 从 metadata 中提取数据，更新 context
			carr := &carrier.GRPCMetadata{MD: md}
			for _, prop := range props {
				ctx = prop.Extract(ctx, carr)
			}
		}

		// 包装 ServerStream，使用新的 context
		wrappedStream := &wrappedServerStream{
			ServerStream: ss,
			ctx:          ctx,
		}

		// 调用下游 handler
		return handler(srv, wrappedStream)
	}
}
