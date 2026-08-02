package discovery_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/izliang/bore/discovery"
)

func TestParseAddress(t *testing.T) {
	tests := []struct {
		addr     string
		wantHost string
		wantPort int
	}{
		{"127.0.0.1:8080", "127.0.0.1", 8080},
		{"0.0.0.0:9090", "0.0.0.0", 9090},
		{"localhost:3000", "localhost", 3000},
		{"[::1]:50051", "::1", 50051},
		{"noport", "noport", 0},
		{"", "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			host, port := discovery.ParseAddress(tt.addr)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantPort, port)
		})
	}
}

func TestResolveAddress_ReplacesZero(t *testing.T) {
	host, port := discovery.ResolveAddress("0.0.0.0:1234")
	assert.Equal(t, 1234, port)
	assert.NotEqual(t, "0.0.0.0", host)
	assert.NotEmpty(t, host)
}

func TestResolveAddress_EmptyHost(t *testing.T) {
	host, port := discovery.ResolveAddress(":1234")
	assert.Equal(t, 1234, port)
	assert.NotEmpty(t, host)
}

func TestResolveAddress_KeepRealIP(t *testing.T) {
	host, port := discovery.ResolveAddress("10.0.0.1:9999")
	assert.Equal(t, "10.0.0.1", host)
	assert.Equal(t, 9999, port)
}

func TestValidateService_Success(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Name:        "test-svc",
		Address:     "0.0.0.0:8080",
		ServiceType: discovery.ServiceTypeHTTP,
	})
	require.NoError(t, err)
}

func TestValidateService_GRPC(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Name:        "grpc-svc",
		Address:     "0.0.0.0:50051",
		ServiceType: discovery.ServiceTypeGRPC,
	})
	require.NoError(t, err)
}

func TestValidateService_EmptyName(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Address:     "0.0.0.0:8080",
		ServiceType: discovery.ServiceTypeHTTP,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name")
}

func TestValidateService_EmptyAddress(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Name:        "svc",
		ServiceType: discovery.ServiceTypeHTTP,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "address")
}

func TestValidateService_InvalidPort(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Name:        "svc",
		Address:     "localhost",
		ServiceType: discovery.ServiceTypeHTTP,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid address")
}

func TestValidateService_BadType(t *testing.T) {
	err := discovery.ValidateService(discovery.Service{
		Name:        "svc",
		Address:     "0.0.0.0:8080",
		ServiceType: "tcp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestEnsurePort_AddsDefault(t *testing.T) {
	assert.Equal(t, "localhost:8500", discovery.EnsurePort("localhost", 8500))
}

func TestEnsurePort_KeepsExisting(t *testing.T) {
	assert.Equal(t, "localhost:9999", discovery.EnsurePort("localhost:9999", 8500))
}

func TestConsulConfig_CreateRegistry(t *testing.T) {
	reg := discovery.NewConsulRegistry(discovery.ConsulConfig{
		Address: "127.0.0.1:8500",
	}, nil)
	require.NotNil(t, reg)
}

func TestConsulLockConfig_Fields(t *testing.T) {
	cfg := discovery.ConsulLockConfig{Key: "test/lock"}
	assert.Equal(t, "test/lock", cfg.Key)
	assert.Equal(t, time.Duration(0), cfg.LockDelay)
	assert.Equal(t, time.Duration(0), cfg.TTL)
	assert.Equal(t, time.Duration(0), cfg.RenewInterval)
}

func TestResolveAdvertiseAddr(t *testing.T) {
	addr := discovery.ResolveAdvertiseAddr()
	assert.NotEmpty(t, addr)
}

func TestNewConsulRegistry(t *testing.T) {
	reg := discovery.NewConsulRegistry(discovery.ConsulConfig{
		Address: "127.0.0.1:8500",
		Scheme:  "https",
		Token:   "secret",
		HealthCheck: discovery.HealthCheckConfig{
			Interval:                       "5s",
			Timeout:                        "3s",
			DeregisterCriticalServiceAfter: "15s",
			HTTPPath:                       "/custom",
		},
	}, nil)
	assert.NotNil(t, reg)
}

func TestNewConsulRegistry_DefaultScheme(t *testing.T) {
	reg := discovery.NewConsulRegistry(discovery.ConsulConfig{
		Address: "127.0.0.1:8500",
	}, nil)
	assert.NotNil(t, reg)
}

func TestServiceType_Constants(t *testing.T) {
	assert.Equal(t, discovery.ServiceType("grpc"), discovery.ServiceTypeGRPC)
	assert.Equal(t, discovery.ServiceType("http"), discovery.ServiceTypeHTTP)
}
