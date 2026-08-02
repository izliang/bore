package interceptor

import (
	"context"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryClientInterceptor gRPC 一元客户端拦截器
// 用于在发送请求前将 context 中的透传数据注入到 gRPC metadata
func UnaryClientInterceptor(props ...propagator.Propagator) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
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

		// 调用下游
		return invoker(newCtx, method, req, reply, cc, opts...)
	}
}

// UnaryServerInterceptor gRPC 一元服务端拦截器
// 用于从接收的 gRPC metadata 中提取透传数据，放入 context
func UnaryServerInterceptor(props ...propagator.Propagator) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		// 从 incoming context 中获取 metadata
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			// 从 metadata 中提取数据，更新 context
			carr := &carrier.GRPCMetadata{MD: md}
			for _, prop := range props {
				ctx = prop.Extract(ctx, carr)
			}
		}

		// 调用下游 handler
		return handler(ctx, req)
	}
}
