package discovery

import "time"

// ConsulConfig Consul 连接与健康检查配置。
//
// 所有字段均由调用方显式传入，不内置任何 config.Get* 或 os.Getenv 调用。
type ConsulConfig struct {
	// Address Consul 地址，如“127.0.0.1:8500”。
	Address string

	// Scheme 协议，默认 “http”。
	Scheme string

	// Token ACL token（可选）。
	Token string

	// Namespace 环境命名空间前缀（可选），用于多环境隔离。
	// 如 "prod.", "staging.", "dev."，注册时会自动添加到服务名前。
	// 例：Namespace="prod.", ServiceName="user" -> 实际注册为 "prod.user"
	Namespace string

	// EnvTag 环境标签（可选），用于按环境过滤服务。
	// 如 "env:prod", "env:staging"，会自动添加到服务 Tags 中。
	EnvTag string

	// HealthCheck 健康检查默认配置，可由 Service.HealthCheck 覆盖。
	HealthCheck HealthCheckConfig
}

// HealthCheckConfig Consul 健康检查配置。
//
// 字段与 Consul AgentServiceCheck 一一对应。
type HealthCheckConfig struct {
	// Interval 检查间隔，如 “10s”。
	Interval string

	// Timeout 检查超时，如 “5s”。
	Timeout string

	// DeregisterCriticalServiceAfter 失败后注销时间，如 “30s”。
	DeregisterCriticalServiceAfter string

	// HTTPPath HTTP 健康检查路径，默认 “/health”。
	HTTPPath string

	// GRPCUseTLS gRPC 健康检查是否使用 TLS。
	GRPCUseTLS bool

	// TTL TTL 模式检查间隔（可选，与 Interval/Timeout 互斥）。
	TTL time.Duration
}

// defaults 填充默认值。
func (c *ConsulConfig) defaults() {
	if c.Scheme == "" {
		c.Scheme = "http"
	}
}

// effectiveHealthCheck 返回有效的健康检查配置。
// svcHC 为非零 HealthCheckConfig 时优先使用，否则回退到 ConsulConfig 中的配置。
func (c *ConsulConfig) effectiveHealthCheck(svcHC *HealthCheckConfig) HealthCheckConfig {
	if svcHC != nil {
		return *svcHC
	}
	return c.HealthCheck
}

// ConsulLockConfig Consul 分布式锁配置。
type ConsulLockConfig struct {
	// Key 锁的 KV 路径。
	Key string

	// LockDelay 获取锁失败后的重试间隔，默认 100ms。
	LockDelay time.Duration

	// TTL Session TTL，默认 30s。
	TTL time.Duration

	// RenewInterval 续约间隔，默认 10s（必须小于 TTL）。
	RenewInterval time.Duration
}

// defaults 填充默认值。
func (c *ConsulLockConfig) defaults() {
	if c.LockDelay == 0 {
		c.LockDelay = 100 * time.Millisecond
	}
	if c.TTL == 0 {
		c.TTL = 30 * time.Second
	}
	if c.RenewInterval == 0 {
		c.RenewInterval = 10 * time.Second
	}
}
