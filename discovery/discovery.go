package discovery

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// ServiceType 服务类型。
type ServiceType string

const (
	ServiceTypeGRPC ServiceType = "grpc"
	ServiceTypeHTTP ServiceType = "http"
)

// Service 表示一个待注册的服务。
type Service struct {
	Name        string
	Address     string // host:port，如 "0.0.0.0:8080"
	Tags        []string
	ServiceType ServiceType
	// HealthCheck 可选覆盖全局健康检查配置。为 nil 时使用 ConsulConfig 中的默认配置。
	HealthCheck *HealthCheckConfig
}

// ServiceInstance 表示一个已注册的服务实例。
type ServiceInstance struct {
	Name    string
	Address string
	Port    int
	Tags    []string
}

// Registry 服务注册与发现接口。
type Registry interface {
	Register(ctx context.Context, svc Service) error
	Deregister(ctx context.Context, svc Service) error
	Discover(ctx context.Context, name string) ([]ServiceInstance, error)
}

// ResolveAdvertiseAddr 解析广告地址。
//
// 优先级：POD_IP 环境变量 > 网络探测 > "127.0.0.1"
func ResolveAdvertiseAddr() string {
	if ip := os.Getenv("POD_IP"); ip != "" {
		return ip
	}
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

// ParseAddress 拆分 host:port 格式的地址。
//
// 返回 host 和 port。无法解析时 port 返回 0。
func ParseAddress(addr string) (string, int) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return addr, 0
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return host, 0
	}
	return host, port
}

// ResolveAddress 解析服务地址，将 0.0.0.0 或空字符串替换为实际 IP。
func ResolveAddress(addr string) (string, int) {
	host, port := ParseAddress(addr)
	if host == "0.0.0.0" || host == "" {
		host = ResolveAdvertiseAddr()
	}
	return host, port
}

// ValidateService 校验服务配置。
func ValidateService(svc Service) error {
	if svc.Name == "" {
		return fmt.Errorf("discovery: service name is required")
	}
	if svc.Address == "" {
		return fmt.Errorf("discovery: service address is required")
	}
	if svc.ServiceType != ServiceTypeGRPC && svc.ServiceType != ServiceTypeHTTP {
		return fmt.Errorf("discovery: unsupported service type: %s", svc.ServiceType)
	}
	// validate port
	_, port := ParseAddress(svc.Address)
	if port == 0 {
		return fmt.Errorf("discovery: invalid address format, expected host:port: %s", svc.Address)
	}
	return nil
}

// EnsurePort 确保地址包含端口。若无端口则追加默认端口。
func EnsurePort(addr string, defaultPort int) string {
	if strings.Contains(addr, ":") {
		return addr
	}
	return fmt.Sprintf("%s:%d", addr, defaultPort)
}
