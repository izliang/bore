package logit

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AccessMiddleware 返回一个 Gin 中间件，在处理器完成后为每个请求记录一条结构化访问日志。
// 它在路径、查询和跳过规则语义上与 gin.LoggerWithConfig 保持一致，但不运行 Gin 的默认文本格式化器
// 也不向 io.Writer 执行 fmt.Fprint——仅输出结构化字段。这避免了每次请求的字符串格式化和丢弃输出端
// （在 Formatter+Discard vs 自定义中间件中性能最优）。
//
// 默认严重级别映射：
//   - status >= 500 → Error
//   - status >= 400 → Warn
//   - 其他 → Info
//
// cfg == nil 时使用默认值。可使用 GinAccessConfig 设置 SkipPaths、SkipQueryString、Skip 或自定义 LevelForStatus。
func AccessMiddleware(log Logger, cfg *GinAccessConfig) gin.HandlerFunc {
	if cfg == nil {
		cfg = &GinAccessConfig{}
	}

	var skip map[string]struct{}
	if n := len(cfg.SkipPaths); n > 0 {
		skip = make(map[string]struct{}, n)
		for _, p := range cfg.SkipPaths {
			skip[p] = struct{}{}
		}
	}

	levelFn := cfg.LevelForStatus
	if levelFn == nil {
		levelFn = defaultGinAccessLevel
	}

	return func(c *gin.Context) {
		start := time.Now()
		pathOnly := c.Request.URL.Path
		rawQuery := c.Request.URL.RawQuery

		c.Next()

		if _, ok := skip[pathOnly]; ok || (cfg.Skip != nil && cfg.Skip(c)) {
			return
		}

		latency := time.Since(start)
		method := c.Request.Method
		status := c.Writer.Status()
		if status == 0 {
			status = http.StatusOK
		}

		logPath := pathOnly
		if rawQuery != "" && !cfg.SkipQueryString {
			logPath = pathOnly + "?" + rawQuery
		}

		bodySz := c.Writer.Size()
		if bodySz < 0 {
			bodySz = 0
		}

		fields := []FieldFunc{
			StringField("method", method),
			StringField("path", logPath),
			IntField("status", status),
			TimeDurationField("latency", latency),
			StringField("client_ip", c.ClientIP()),
			IntField("body_size", bodySz),
		}

		if msg := c.Errors.ByType(gin.ErrorTypePrivate).String(); msg != "" {
			fields = append(fields, StringField("gin_errors", msg))
		}

		msg, sev := levelFn(status)
		ctx := requestContext(c)
		switch sev {
		case GinAccessError:
			log.Error(ctx, fmt.Errorf("%s: HTTP %d", msg, status), fields...)
		case GinAccessWarn:
			log.Warn(ctx, msg, fields...)
		case GinAccessInfo:
			log.Info(ctx, msg, fields...)
		case GinAccessDebug:
			log.Debug(ctx, msg, fields...)
		default:
			log.Info(ctx, msg, fields...)
		}
	}
}

// GinAccessConfig 配置 AccessMiddleware。零值表示：记录所有路径、路径包含查询字符串、无跳过函数。
type GinAccessConfig struct {
	// SkipPaths 在 Request.URL.Path 完全匹配时抑制日志（同 gin.LoggerConfig.SkipPaths）。
	SkipPaths []string
	// SkipQueryString 为 true 时从日志路径中省略查询字符串（同 gin.LoggerConfig.SkipQueryString）。
	SkipQueryString bool
	// Skip 是可选的每次请求过滤器（同 gin.LoggerConfig.Skip）。
	Skip gin.Skipper
	// LevelForStatus 将 HTTP 状态码映射为日志消息和严重级别。若为 nil，使用内置映射。
	LevelForStatus func(status int) (msg string, sev GinAccessSeverity)
}

// GinAccessSeverity 选择 AccessMiddleware 调用的 Logger 方法。
type GinAccessSeverity int

const (
	// GinAccessDebug 映射到 Logger.Debug。
	GinAccessDebug GinAccessSeverity = iota
	// GinAccessInfo 映射到 Logger.Info。
	GinAccessInfo
	// GinAccessWarn 映射到 Logger.Warn。
	GinAccessWarn
	// GinAccessError 映射到 Logger.Error（从消息和状态码构造非 nil error）。
	GinAccessError
)

func defaultGinAccessLevel(status int) (string, GinAccessSeverity) {
	const msg = "http_access"
	switch {
	case status >= http.StatusInternalServerError:
		return msg, GinAccessError
	case status >= http.StatusBadRequest:
		return msg, GinAccessWarn
	default:
		return msg, GinAccessInfo
	}
}

func requestContext(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

// ErrorMiddleware 在处理器运行后将 c.Errors 中追加的每条错误（所有 gin.Error 条目）记录下来。
// 它不格式化 HTTP 响应。适用于需要将 Bind/render 错误记录到结构化日志中的场景；
// 若与 AccessMiddleware 中的 gin_errors 同时启用可能造成重复，应避免一起使用。
func ErrorMiddleware(log Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		ctx := requestContext(c)
		for _, e := range c.Errors {
			if e == nil {
				continue
			}
			errStr := ""
			if e.Err != nil {
				errStr = e.Err.Error()
			}
			log.Warn(ctx, "gin_context_error",
				StringField("gin_error_type", fmt.Sprintf("0x%x", uint64(e.Type))),
				StringField("err", errStr),
			)
		}
	}
}

type GinLogWriter struct {
	logger Logger
	buf    bytes.Buffer
}

func NewGinLogWriter(logger Logger) *GinLogWriter {
	return &GinLogWriter{logger: logger}
}

func (w *GinLogWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	for _, b := range p {
		if b == '\n' {
			line := strings.TrimRight(w.buf.String(), "\r")
			w.buf.Reset()
			if line != "" {
				w.logger.Info(context.Background(), line)
			}
		} else {
			w.buf.WriteByte(b)
		}
	}
	return n, nil
}
