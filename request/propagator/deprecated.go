// Package propagator 已废弃，请使用 git.100credit.cn/.../propagator 及其子包。
//
// 迁移映射：
//
//	旧 API                                    新 API
//	──────────────────────────────────────    ──────────────────────────────
//	propagator.Propagator (接口)              propagator.Propagator (根包)
//	propagator.NewSREPropagator(opts...)      sre.New(opts...)
//	propagator.SetSREPropagator(ctx, data)    sre.Set(ctx, data)
//	propagator.GetSREPropagator(ctx)          sre.Get(ctx)
//	propagator.WithSREPropagatorKey(key)      sre.WithKey(key)
//	propagator.WithSREPropagatorMap(k, v)     sre.WithMap(k, v)
//	propagator.SREPropagatorData              sre.SREPropagatorData
//	propagator.SREPropagatorItem              sre.SREPropagatorItem
//
//	Propagator 接口方法变更：
//	InjectHTTP(ctx, header)                  Inject(ctx, &carrier.HTTPHeader{Header: header})
//	ExtractHTTP(ctx, header)                 Extract(ctx, &carrier.HTTPHeader{Header: header})
//	InjectGRPC(ctx, md)                      Inject(ctx, &carrier.GRPCMetadata{MD: md})
//	ExtractGRPC(ctx, md)                     Extract(ctx, &carrier.GRPCMetadata{MD: md})
//	InjectMQ(ctx, headers)                   Inject(ctx, &carrier.MQHeaders{Headers: headers})
//	ExtractMQ(ctx, headers)                  Extract(ctx, &carrier.MQHeaders{Headers: headers})
//
// 详细迁移指南见项目根目录 MIGRATION.md。
package propagator

import (
	"context"
	"net/http"

	"github.com/izliang/bore/propagator"
	"github.com/izliang/bore/propagator/carrier"
	sre "github.com/izliang/bore/propagator/sre"
	"google.golang.org/grpc/metadata"
)

// ──────────────────────────── 接口（弃用） ────────────────────────────

// Deprecated: 请使用 propagator.Propagator（根包）。
// Propagator 接口已简化为 Inject(ctx, carrier) / Extract(ctx, carrier)，不再区分协议。
//
//	// 旧用法
//	prop.InjectHTTP(ctx, header)
//
//	// 新用法
//	prop.Inject(ctx, &carrier.HTTPHeader{Header: header})
type Propagator = propagator.Propagator

// ──────────────────────────── 构造 & 类型（弃用） ─────────────────────

// Deprecated: 请使用 sre.New(opts...)。
var NewSREPropagator = sre.New

// Deprecated: 请使用 sre.WithKey(key)。
var WithSREPropagatorKey = sre.WithKey

// Deprecated: 请使用 sre.WithMap(key, value)。
var WithSREPropagatorMap = sre.WithMap

// Deprecated: 请使用 sre.SREPropagatorData。
type SREPropagatorData = sre.SREPropagatorData

// Deprecated: 请使用 sre.SREPropagatorItem。
type SREPropagatorItem = sre.SREPropagatorItem

// Deprecated: 请使用 sre.Set(ctx, data)。
var SetSREPropagator = sre.Set

// Deprecated: 请使用 sre.Get(ctx)。
var GetSREPropagator = sre.Get

// ──────────────────────────── 便捷适配器（弃用） ──────────────────────

// Deprecated: 请直接使用 propagator.Propagator.Inject(ctx, carrier)。
// InjectHTTP 是对 Inject + HTTPHeader 的便捷封装，仅用于迁移过渡。
func InjectHTTP(p propagator.Propagator, ctx context.Context, header http.Header) {
	p.Inject(ctx, &carrier.HTTPHeader{Header: header})
}

// Deprecated: 请直接使用 propagator.Propagator.Extract(ctx, carrier)。
func ExtractHTTP(p propagator.Propagator, ctx context.Context, header http.Header) context.Context {
	return p.Extract(ctx, &carrier.HTTPHeader{Header: header})
}

// Deprecated: 请直接使用 propagator.Propagator.Inject(ctx, carrier)。
func InjectGRPC(p propagator.Propagator, ctx context.Context, md metadata.MD) {
	p.Inject(ctx, &carrier.GRPCMetadata{MD: md})
}

// Deprecated: 请直接使用 propagator.Propagator.Extract(ctx, carrier)。
func ExtractGRPC(p propagator.Propagator, ctx context.Context, md metadata.MD) context.Context {
	return p.Extract(ctx, &carrier.GRPCMetadata{MD: md})
}

// Deprecated: 请直接使用 propagator.Propagator.Inject(ctx, carrier)。
func InjectMQ(p propagator.Propagator, ctx context.Context, headers map[string]string) {
	p.Inject(ctx, &carrier.MQHeaders{Headers: headers})
}

// Deprecated: 请直接使用 propagator.Propagator.Extract(ctx, carrier)。
func ExtractMQ(p propagator.Propagator, ctx context.Context, headers map[string]string) context.Context {
	return p.Extract(ctx, &carrier.MQHeaders{Headers: headers})
}
