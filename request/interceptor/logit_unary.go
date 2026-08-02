package interceptor

import (
	"context"

	"github.com/izliang/bore/logit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func LogitUnaryClientInterceptor(logger logit.Logger) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		md, ok := metadata.FromOutgoingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		logger.Info(ctx, "grpc unary client metadata",
			logit.StringField("method", method),
			logit.JSONAbsField("metadata", md),
		)

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func LogitUnaryServerInterceptor(logger logit.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			md = metadata.New(nil)
		}

		method := ""
		if info != nil {
			method = info.FullMethod
		}
		logger.Info(ctx, "grpc unary server metadata",
			logit.StringField("method", method),
			logit.JSONAbsField("metadata", md),
		)

		return handler(ctx, req)
	}
}
