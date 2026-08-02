// Package router 提供基于 Gin 框架的路由注册封装，支持两种模式：
//
//   - simple 模式（simple/ 子包）：纯 Gin，零 Huma 依赖，适合内部微服务
//   - huma 模式（huma/ 子包）：Huma v2 集成，自动 OpenAPI 文档和校验
//
// 本包提供两个子包共享的类型和工具函数：
//   - MetricsReporter — API 指标上报接口
//   - DocConfig / DocExposure / DocUI — 文档暴露策略
//   - HTTPStatusFromError — HTTP 状态码决策
//   - APINameFromPath — 从路径提取 API 名称
//
// 两种模式共享相同的 Handler 签名（define.TypedHandler），实现"一次编写，双模运行"。
package router

import (
	"net/http"
	"time"

	"github.com/izliang/bore/e"
)

// ============================================================
// MetricsReporter — API 指标上报
// ============================================================

// MetricsReporter 可选的 API 调用指标上报接口。
//
// 实现此接口并将实例注入 Router 后，每次 API 调用都会自动上报。
// 如果不需要 Metrics，保持 nil 即可。
type MetricsReporter interface {
	// ObserveAPI 记录一次 API 调用。
	//
	// apiName: 从 URL 路径最后一段提取的 API 名称
	// code: 错误码（成功时为空字符串或 "000000"）
	// duration: 调用耗时（秒）
	// err: 业务错误（成功时为 nil）
	ObserveAPI(apiName string, code string, duration float64, err error)
}

// ============================================================
// HTTP 状态码决策
// ============================================================

// HTTPStatusFromError 根据 e.TypedError 的类型决定 HTTP 状态码。
//
// 规则：
//   - 鉴权错误 → 403 Forbidden
//   - 未找到   → 404 Not Found
//   - 客户端错误 → 400 Bad Request
//   - 其他      → 200 OK（业务错误码由 Envelope.code 表达）
func HTTPStatusFromError(err e.TypedError) int {
	switch {
	case err.IsAuthError():
		return http.StatusForbidden
	case err.IsNotFound():
		return http.StatusNotFound
	case err.IsClientError():
		return http.StatusBadRequest
	default:
		return http.StatusOK
	}
}

// ReportMetricsOnce 上报一次 API 调用指标（供子包通过 MetricsReporter 接口复用）。
func ReportMetricsOnce(m MetricsReporter, apiName, code string, start time.Time, err error) {
	if m == nil {
		return
	}
	m.ObserveAPI(apiName, code, time.Since(start).Seconds(), err)
}

// ============================================================
// 辅助函数
// ============================================================

// APINameFromPath 从路由路径中提取 API 名称（最后一段）。
//
// 示例：
//
//	"/api/v1/users"   → "users"
//	"/api/v1/orders/" → "orders"
//	""                → "unknown"
func APINameFromPath(path string) string {
	if path == "" {
		return "unknown"
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
