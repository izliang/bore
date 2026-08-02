# 百工 gRPC 标准
## SRE 支持
sre 需要 新增svc端口，使其支持 gRPC 的http2协议，SRE默认的http接口不支持http2。
```
  ports:
  - name: http-agent-multi-modal-g
    port: 80
    protocol: TCP
    targetPort: 80
  - name: grpc-agent-multi-modal-g
    port: 8002
    protocol: TCP
    targetPort: 8002
```
## proto 代码生成
示例文件：grpc/proto/user/v1/user.proto

在 grpc 目录下执行
```
protoc --go_out=.  --go-grpc_out=. proto/user/v1/user.proto
```

即可生成 grpc/proto_go/user/v1 文件，以便于 client和server使用

## server 配置
### 启动 gRPC server
```
func startGRPCServer(ctx context.Context, logger logit.Logger) {
	lis, err := net.Listen("tcp", ":8002")
	if err != nil {
		logger.Warn(ctx, "API Listen error: %v", logit.ErrorField("err", err))
	}

	sreProp := propagator.NewSREPropagator(
		propagator.WithSREPropagatorKey("x-request-id"),
		propagator.WithSREPropagatorKey("x-user-id"),
	)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			interceptor.UnaryServerInterceptor(sreProp),
			interceptor.LogitUnaryServerInterceptor(resource.Logger),
		))

	userv1.RegisterUserServiceServer(
		grpcServer,
		&grpc_svc.UserService{
			Logger: logger,
		},
	)

	logger.Info(ctx, "GRPC server listening on :8002")

	if err = grpcServer.Serve(lis); err != nil {
		logger.Warn(ctx, "grpcServer.Serve error: %v", logit.ErrorField("err", err))
	}
}
```
### 业务实现代码
```
package grpc_svc

import (
	"context"

	userv1 "github.com/izliang/bore/grpc/proto_go/user/v1"
	"github.com/izliang/bore/logit"
)

type UserService struct {
	Logger logit.Logger
	userv1.UnimplementedUserServiceServer
}

func (s *UserService) GetUser(
	ctx context.Context,
	req *userv1.GetUserRequest,
) (*userv1.GetUserResponse, error) {
	s.Logger.Info(ctx, "Received GetUser request", logit.JSONAbsField("req", req))
	return &userv1.GetUserResponse{
		UserId: req.UserId,
		Name:   "Tom",
	}, nil
}

```

## 启动 gRPC client
添加 gRPC 透传拦截器实现，以及日志拦截器实现
```
	conn, err := grpc.NewClient(
		"dns:///agent-multi-modal-g:8002",
		grpc.WithTransportCredentials(
			insecure.NewCredentials(),
		),
		grpc.WithChainUnaryInterceptor(
			interceptor.UnaryClientInterceptor(resource.SREPropagator),
			interceptor.LogitUnaryClientInterceptor(logger.GetLogit()),
		),
		grpc.WithChainStreamInterceptor(interceptor.StreamClientInterceptor(resource.SREPropagator)),
	)
	if err != nil {
		return err
	}
	client := userv1.NewUserServiceClient(conn)

	resp, err := client.GetUser(
		c.Request.Context(),
		&userv1.GetUserRequest{
			UserId: 1001,
		},
	)
```