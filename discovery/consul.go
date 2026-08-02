package discovery

import (
	"context"
	"fmt"
	"strings"

	capi "github.com/hashicorp/consul/api"
	"github.com/izliang/bore/logit"
)

// ConsulRegistry 基于 HashiCorp Consul 的服务注册与发现实现。
//
// 实现 Registry 接口。
type ConsulRegistry struct {
	cfg    ConsulConfig
	logger logit.Logger
}

// NewConsulRegistry 创建 Consul 注册中心。
//
// logger 可为 nil，此时不输出日志。
func NewConsulRegistry(cfg ConsulConfig, logger logit.Logger) *ConsulRegistry {
	cfg.defaults()
	return &ConsulRegistry{cfg: cfg, logger: logger}
}

// client 创建 Consul API 客户端。
func (r *ConsulRegistry) client() (*capi.Client, error) {
	return capi.NewClient(&capi.Config{
		Address: r.cfg.Address,
		Scheme:  r.cfg.Scheme,
		Token:   r.cfg.Token,
	})
}

// Register 注册服务。
func (r *ConsulRegistry) Register(ctx context.Context, svc Service) error {
	if err := ValidateService(svc); err != nil {
		return err
	}

	client, err := r.client()
	if err != nil {
		return fmt.Errorf("discovery: create consul client: %w", err)
	}

	host, port := ResolveAddress(svc.Address)
	hc := r.cfg.effectiveHealthCheck(svc.HealthCheck)

	// 应用命名空间前缀到服务名
	serviceName := r.withNamespace(svc.Name)

	// 添加环境标签
	tags := make([]string, 0, len(svc.Tags)+1)
	tags = append(tags, svc.Tags...)
	if r.cfg.EnvTag != "" {
		tags = append(tags, r.cfg.EnvTag)
	}

	registration := &capi.AgentServiceRegistration{
		Name:    serviceName,
		Address: host,
		Port:    port,
		Tags:    tags,
	}

	if err := r.buildHealthCheck(registration, svc.ServiceType, host, port, hc); err != nil {
		return err
	}

	if err := client.Agent().ServiceRegister(registration); err != nil {
		return fmt.Errorf("discovery: register service %s: %w", svc.Name, err)
	}

	r.infof("registered %s service: %s at %s:%d", svc.ServiceType, svc.Name, host, port)
	return nil
}

// Deregister 注销服务。
func (r *ConsulRegistry) Deregister(ctx context.Context, svc Service) error {
	if svc.Name == "" {
		return fmt.Errorf("discovery: service name is required")
	}

	client, err := r.client()
	if err != nil {
		return fmt.Errorf("discovery: create consul client: %w", err)
	}

	// Deregister by service ID: name-address (带命名空间前缀)
	host, port := ResolveAddress(svc.Address)
	serviceName := r.withNamespace(svc.Name)
	serviceID := fmt.Sprintf("%s-%s-%d", serviceName, host, port)

	if err := client.Agent().ServiceDeregister(serviceID); err != nil {
		return fmt.Errorf("discovery: deregister service %s: %w", svc.Name, err)
	}

	r.infof("deregistered %s service: %s at %s:%d", svc.ServiceType, svc.Name, host, port)
	return nil
}

// Discover 发现服务实例（精确匹配）。
func (r *ConsulRegistry) Discover(ctx context.Context, name string) ([]ServiceInstance, error) {
	client, err := r.client()
	if err != nil {
		return nil, fmt.Errorf("discovery: create consul client: %w", err)
	}

	// 应用命名空间前缀
	serviceName := r.withNamespace(name)

	// 构建过滤条件（按服务名）
	filter := fmt.Sprintf("Service == \"%s\"", serviceName)

	services, err := client.Agent().ServicesWithFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("discovery: discover service %s: %w", name, err)
	}

	result := make([]ServiceInstance, 0, len(services))
	for _, s := range services {
		result = append(result, ServiceInstance{
			Name:    r.trimNamespace(s.Service),
			Address: s.Address,
			Port:    s.Port,
			Tags:    s.Tags,
		})
	}
	return result, nil
}

// DiscoverWithPrefix 按前缀发现服务实例（前缀匹配）。
// 例如：prefix="user" 会匹配 "user", "user-api", "user-service" 等
func (r *ConsulRegistry) DiscoverWithPrefix(ctx context.Context, prefix string) ([]ServiceInstance, error) {
	client, err := r.client()
	if err != nil {
		return nil, fmt.Errorf("discovery: create consul client: %w", err)
	}

	// 应用命名空间前缀
	fullPrefix := r.withNamespace(prefix)

	// 获取所有服务，然后按前缀过滤
	// 注意：Consul Agent API 不支持原生前缀匹配，需要先获取所有再过滤
	services, err := client.Agent().Services()
	if err != nil {
		return nil, fmt.Errorf("discovery: list services: %w", err)
	}

	result := make([]ServiceInstance, 0)
	for _, s := range services {
		if strings.HasPrefix(s.Service, fullPrefix) {
			result = append(result, ServiceInstance{
				Name:    r.trimNamespace(s.Service),
				Address: s.Address,
				Port:    s.Port,
				Tags:    s.Tags,
			})
		}
	}
	return result, nil
}

// DiscoverByTag 按 Tag 过滤发现服务实例。
// 支持按环境标签过滤：envTag="env:prod" 只会返回 prod 环境的服务
func (r *ConsulRegistry) DiscoverByTag(ctx context.Context, name string, requiredTag string) ([]ServiceInstance, error) {
	client, err := r.client()
	if err != nil {
		return nil, fmt.Errorf("discovery: create consul client: %w", err)
	}

	// 应用命名空间前缀
	serviceName := r.withNamespace(name)

	// 构建过滤条件（服务名 + Tag）
	filter := fmt.Sprintf("Service == \"%s\" and \"%s\" in Tags", serviceName, requiredTag)

	services, err := client.Agent().ServicesWithFilter(filter)
	if err != nil {
		return nil, fmt.Errorf("discovery: discover service %s with tag %s: %w", name, requiredTag, err)
	}

	result := make([]ServiceInstance, 0, len(services))
	for _, s := range services {
		result = append(result, ServiceInstance{
			Name:    r.trimNamespace(s.Service),
			Address: s.Address,
			Port:    s.Port,
			Tags:    s.Tags,
		})
	}
	return result, nil
}

// RegisterGRPC 注册 gRPC 服务的便捷方法。
func (r *ConsulRegistry) RegisterGRPC(ctx context.Context, name, addr string, tags []string, hc *HealthCheckConfig) error {
	return r.Register(ctx, Service{
		Name:        name,
		Address:     addr,
		Tags:        tags,
		ServiceType: ServiceTypeGRPC,
		HealthCheck: hc,
	})
}

// RegisterHTTP 注册 HTTP 服务的便捷方法。
func (r *ConsulRegistry) RegisterHTTP(ctx context.Context, name, addr, healthPath string, tags []string, hc *HealthCheckConfig) error {
	svcHC := hc
	if healthPath != "" {
		if svcHC == nil {
			svcHC = &HealthCheckConfig{}
		}
		if svcHC.HTTPPath == "" {
			svcHC.HTTPPath = healthPath
		}
	}
	return r.Register(ctx, Service{
		Name:        name,
		Address:     addr,
		Tags:        tags,
		ServiceType: ServiceTypeHTTP,
		HealthCheck: svcHC,
	})
}

// buildHealthCheck 构建 Consul 健康检查配置。
func (r *ConsulRegistry) buildHealthCheck(reg *capi.AgentServiceRegistration, svcType ServiceType, host string, port int, hc HealthCheckConfig) error {
	check := &capi.AgentServiceCheck{
		Interval:                       hc.Interval,
		Timeout:                        hc.Timeout,
		DeregisterCriticalServiceAfter: hc.DeregisterCriticalServiceAfter,
	}

	switch svcType {
	case ServiceTypeGRPC:
		if hc.TTL > 0 {
			check.TTL = hc.TTL.String()
		} else {
			check.GRPC = fmt.Sprintf("%s:%d", host, port)
			check.GRPCUseTLS = hc.GRPCUseTLS
		}
	case ServiceTypeHTTP:
		if hc.TTL > 0 {
			check.TTL = hc.TTL.String()
		} else {
			path := hc.HTTPPath
			if path == "" {
				path = "/health"
			}
			check.HTTP = fmt.Sprintf("http://%s:%d%s", host, port, path)
		}
	default:
		return fmt.Errorf("discovery: unsupported service type: %s", svcType)
	}

	reg.Check = check
	return nil
}

func (r *ConsulRegistry) infof(format string, args ...interface{}) {
	if r.logger != nil {
		r.logger.Info(context.Background(), fmt.Sprintf(format, args...))
	}
}

// withNamespace 给服务名添加命名空间前缀。
func (r *ConsulRegistry) withNamespace(name string) string {
	if r.cfg.Namespace == "" {
		return name
	}
	return r.cfg.Namespace + name
}

// trimNamespace 移除服务名的命名空间前缀。
func (r *ConsulRegistry) trimNamespace(name string) string {
	if r.cfg.Namespace == "" {
		return name
	}
	return strings.TrimPrefix(name, r.cfg.Namespace)
}

// 确保实现 Registry 接口
var _ Registry = (*ConsulRegistry)(nil)
