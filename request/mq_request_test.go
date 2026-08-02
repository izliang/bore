package request

import (
	"context"
	"testing"

	"github.com/izliang/bore/propagator/carrier"
	sre "github.com/izliang/bore/propagator/sre"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSREPropagator_InjectMQ(t *testing.T) {
	t.Parallel()

	ctx := sre.Set(context.Background(), sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
			{ServerName: "credit", BrhGrayVersion: "v2"},
		},
	})
	p := sre.New(
		sre.WithMap("x-request-id", "req-123"),
	)
	headers := make(map[string]string)

	p.Inject(ctx, &carrier.MQHeaders{Headers: headers})

	assert.Equal(t, "prod", headers["x-sre-env"])
	assert.Equal(t, "v1", headers["brh-gray-risk"])
	assert.Equal(t, "20240520", headers["brh-subversion-risk"])
	assert.Equal(t, "v2", headers["brh-gray-credit"])
	assert.Equal(t, "req-123", headers["x-request-id"])
}

func TestSREPropagator_ExtractMQ(t *testing.T) {
	t.Parallel()

	headers := map[string]string{
		"X-Sre-Env":           "test",
		"Brh-Gray-Risk":       "v1",
		"Brh-Subversion-Risk": "20240520",
		"BRH-GRAY-CREDIT":     "v2",
	}
	p := sre.New(
		sre.WithMap("x-request-id", ""),
	)
	headers["X-Request-Id"] = "req-123"

	ctx := p.Extract(context.Background(), &carrier.MQHeaders{Headers: headers})
	actual := sre.Get(ctx)

	require.NotNil(t, actual)
	assert.Equal(t, "test", actual.Env)

	apps := make(map[string]sre.SREPropagatorItem)
	for _, app := range actual.Apps {
		apps[app.ServerName] = app
	}
	assert.Equal(t, "v1", apps["risk"].BrhGrayVersion)
	assert.Equal(t, "20240520", apps["risk"].BrhSubVersion)
	assert.Equal(t, "v2", apps["credit"].BrhGrayVersion)
}

func TestSREPropagator_MQRoundTrip(t *testing.T) {
	t.Parallel()

	original := sre.SREPropagatorData{
		Env: "prod",
		Apps: []sre.SREPropagatorItem{
			{ServerName: "risk", BrhGrayVersion: "v1", BrhSubVersion: "20240520"},
			{ServerName: "credit", BrhGrayVersion: "v2", BrhSubVersion: "20240521"},
		},
	}
	ctx := sre.Set(context.Background(), original)
	headers := make(map[string]string)
	p := sre.New()

	p.Inject(ctx, &carrier.MQHeaders{Headers: headers})
	extractedCtx := p.Extract(context.Background(), &carrier.MQHeaders{Headers: headers})
	extracted := sre.Get(extractedCtx)

	require.NotNil(t, extracted)
	assert.Equal(t, original.Env, extracted.Env)
	assert.Len(t, extracted.Apps, len(original.Apps))

	apps := make(map[string]sre.SREPropagatorItem)
	for _, app := range extracted.Apps {
		apps[app.ServerName] = app
	}
	for _, app := range original.Apps {
		assert.Equal(t, app.BrhGrayVersion, apps[app.ServerName].BrhGrayVersion)
		assert.Equal(t, app.BrhSubVersion, apps[app.ServerName].BrhSubVersion)
	}
}

func TestSREPropagator_MQExtraRoundTrip(t *testing.T) {
	t.Parallel()

	p := sre.New(
		sre.WithKey("x-request-id"),
		sre.WithMap("x-tenant-id", "default-tenant"),
	)
	headers := map[string]string{
		"X-Sre-Env":    "prod",
		"X-Request-Id": "req-123",
		"X-Tenant-Id":  "tenant-456",
	}

	ctx := p.Extract(context.Background(), &carrier.MQHeaders{Headers: headers})
	data := sre.Get(ctx)
	require.NotNil(t, data)
	assert.Equal(t, map[string]string{
		"x-request-id": "req-123",
		"x-tenant-id":  "tenant-456",
	}, data.Extra)

	outgoing := make(map[string]string)
	p.Inject(ctx, &carrier.MQHeaders{Headers: outgoing})

	assert.Equal(t, "prod", outgoing["x-sre-env"])
	assert.Equal(t, "req-123", outgoing["x-request-id"])
	assert.Equal(t, "tenant-456", outgoing["x-tenant-id"])
}
