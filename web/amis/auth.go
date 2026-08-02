package amis

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

// ============================================================
// AuthConfig — 鉴权配置
// ============================================================

// AuthConfig 定义 amis 页面的鉴权行为配置。
//
// 所有字段均可选，未设置时使用 DefaultAuthConfig() 的默认值。
type AuthConfig struct {
	// LoginPath 登录页面 URL 路径（默认 "/login"）。
	LoginPath string

	// LoginAPIPath 登录 API 端点（默认 "/api/login"），
	// 前端登录表单会 POST 到此端点。
	LoginAPIPath string

	// LogoutAPIPath 登出 API 端点（默认 "/api/logout"）。
	LogoutAPIPath string

	// HomePath 登录成功后的默认跳转路径（默认 "/"）。
	// 若 URL 中有 ?redirect= 参数，则优先级更高。
	HomePath string

	// CookieName 鉴权 Cookie 名称（默认 "amis_token"）。
	CookieName string

	// TokenLookup 自定义 Token 提取逻辑。
	// 若为 nil，默认从 Cookie 中读取（CookieName）。
	// 可实现从 Authorization header、URL 参数等提取 Token。
	TokenLookup func(c *gin.Context) (token string, ok bool)

	// Skipper 自定义跳过鉴权的路径判断。
	// 返回 true 表示跳过鉴权检查。
	// 若为 nil，默认跳过 LoginPath、LoginAPIPath、LogoutAPIPath。
	Skipper func(c *gin.Context) bool

	// Messages 鉴权流程中的用户界面文案。
	// 若为 nil，使用 DefaultAuthMessages()（英文）。
	// 可传入 AuthMessagesZH() 切换到中文。
	Messages *AuthMessages
}

// DefaultAuthConfig 返回生产安全的默认鉴权配置。
func DefaultAuthConfig() AuthConfig {
	return AuthConfig{
		LoginPath:     "/login",
		LoginAPIPath:  "/api/login",
		LogoutAPIPath: "/api/logout",
		HomePath:      "/",
		CookieName:    "amis_token",
	}
}

// normalize 填充未设置的字段为默认值。
func (c *AuthConfig) normalize() {
	if c.LoginPath == "" {
		c.LoginPath = "/login"
	}
	if c.LoginAPIPath == "" {
		c.LoginAPIPath = "/api/login"
	}
	if c.LogoutAPIPath == "" {
		c.LogoutAPIPath = "/api/logout"
	}
	if c.HomePath == "" {
		c.HomePath = "/"
	}
	if c.CookieName == "" {
		c.CookieName = "amis_token"
	}
	if c.Messages == nil {
		m := DefaultAuthMessages()
		c.Messages = &m
	}
	c.Messages.normalize()
}

// ============================================================
// Authenticator — 凭证验证回调
// ============================================================

// Authenticator 是用户凭证验证函数签名。
//
// 由调用方实现具体的密码校验逻辑（数据库查询、LDAP、OAuth 等）。
//
// 参数：
//   - username: 登录表单提交的用户名
//   - password: 登录表单提交的密码
//
// 返回值：
//   - token:    鉴权令牌（将写入 Cookie）
//   - redirect: 登录后的跳转路径（空字符串表示使用 URL ?redirect 参数或 HomePath）
//   - err:      非 nil 表示验证失败，错误信息会返回给前端
type Authenticator func(username, password string) (token string, redirect string, err error)

// ============================================================
// LoginSchema — 登录页面 Schema 生成
// ============================================================

// LoginSchema 生成 amis 登录页面的 Schema（JSON 对象）。
//
// 生成的页面包含用户名 + 密码输入框，登录成功后自动跳转。
func (c AuthConfig) LoginSchema() map[string]interface{} {
	c.normalize()
	m := c.Messages

	return map[string]interface{}{
		"type":  "page",
		"title": m.LoginTitle,
		"body": map[string]interface{}{
			"type": "wrapper",
			"style": map[string]interface{}{
				"maxWidth":     "400px",
				"margin":       "120px auto 0",
				"padding":      "40px",
				"background":   "#fff",
				"borderRadius": "8px",
				"boxShadow":    "0 2px 16px rgba(0,0,0,0.1)",
			},
			"body": map[string]interface{}{
				"type":  "form",
				"title": "",
				"mode":  "horizontal",
				"api": map[string]interface{}{
					"method": "post",
					"url":    c.LoginAPIPath,
				},
				"body": []interface{}{
					map[string]interface{}{
						"type":        "input-text",
						"name":        "username",
						"label":       m.UsernameLabel,
						"placeholder": m.UsernamePlaceholder,
						"required":    true,
						"size":        "full",
					},
					map[string]interface{}{
						"type":        "input-password",
						"name":        "password",
						"label":       m.PasswordLabel,
						"placeholder": m.PasswordPlaceholder,
						"required":    true,
						"size":        "full",
					},
					map[string]interface{}{
						"type":       "button",
						"label":      m.LoginButton,
						"level":      "primary",
						"block":      true,
						"size":       "lg",
						"actionType": "submit",
					},
				},
				"onEvent": map[string]interface{}{
					"submitSucc": map[string]interface{}{
						"actions": []interface{}{
							map[string]interface{}{
								"actionType": "url",
								"args": map[string]interface{}{
									"url":   "${event.data.result.data.redirect || '" + c.HomePath + "'}",
									"blank": false,
								},
							},
						},
					},
				},
			},
		},
	}
}

// ============================================================
// RequireAuth — Gin 鉴权中间件
// ============================================================

// RequireAuth 返回 Gin 鉴权中间件。
//
// 行为：
//   - 跳过 Skipper 指定的路径（默认跳过 LoginPath/LoginAPIPath/LogoutAPIPath）
//   - 对浏览器页面请求（Accept: text/html + GET）：302 重定向到登录页，携带 ?redirect=
//   - 对 API 请求（XHR/fetch）：返回 401 JSON
func (c AuthConfig) RequireAuth() gin.HandlerFunc {
	c.normalize()

	defaultSkipper := func(ctx *gin.Context) bool {
		p := ctx.Request.URL.Path
		return p == c.LoginPath || p == c.LoginAPIPath || p == c.LogoutAPIPath
	}

	return func(ctx *gin.Context) {
		skip := defaultSkipper(ctx)
		if c.Skipper != nil {
			skip = c.Skipper(ctx)
		}
		if skip {
			ctx.Next()
			return
		}

		if ok := c.hasValidToken(ctx); ok {
			ctx.Next()
			return
		}

		c.abortUnauthorized(ctx)
	}
}

func (c *AuthConfig) hasValidToken(ctx *gin.Context) bool {
	token, ok := c.lookupToken(ctx)
	return ok && token != ""
}

func (c *AuthConfig) lookupToken(ctx *gin.Context) (string, bool) {
	if c.TokenLookup != nil {
		return c.TokenLookup(ctx)
	}
	token, err := ctx.Cookie(c.CookieName)
	if err != nil {
		return "", false
	}
	return token, true
}

func (c *AuthConfig) abortUnauthorized(ctx *gin.Context) {
	if isPageRequest(ctx) {
		redirectTarget := ctx.Request.URL.Path
		if ctx.Request.URL.RawQuery != "" {
			redirectTarget += "?" + ctx.Request.URL.RawQuery
		}
		loginURL := c.LoginPath + "?redirect=" + url.QueryEscape(redirectTarget)
		ctx.Redirect(http.StatusFound, loginURL)
		ctx.Abort()
		return
	}

	ctx.JSON(http.StatusUnauthorized, gin.H{
		"status": 401,
		"msg":    c.Messages.ErrUnauthorized,
		"data":   nil,
	})
	ctx.Abort()
}

func isPageRequest(ctx *gin.Context) bool {
	if ctx.Request.Method != http.MethodGet {
		return false
	}
	accept := ctx.GetHeader("Accept")
	return strings.Contains(accept, "text/html")
}

// ============================================================
// LoginHandler / LogoutHandler
// ============================================================

// LoginHandler 返回登录 API 的 Gin Handler。
func (c AuthConfig) LoginHandler(auth Authenticator) gin.HandlerFunc {
	c.normalize()

	return func(ctx *gin.Context) {
		var req struct {
			Username string `json:"username" form:"username"`
			Password string `json:"password" form:"password"`
		}
		if err := ctx.ShouldBind(&req); err != nil || req.Username == "" || req.Password == "" {
			ctx.JSON(http.StatusOK, gin.H{
				"status": 422,
				"msg":    c.Messages.ErrMissingCredentials,
				"data":   nil,
			})
			return
		}

		token, redirect, err := auth(req.Username, req.Password)
		if err != nil {
			ctx.JSON(http.StatusOK, gin.H{
				"status": 422,
				"msg":    err.Error(),
				"data":   nil,
			})
			return
		}

		if redirect == "" {
			redirect = ctx.Query("redirect")
		}
		if redirect == "" {
			redirect = c.HomePath
		}

		ctx.SetCookie(c.CookieName, token, 86400, "/", "", false, true)

		ctx.JSON(http.StatusOK, gin.H{
			"status": 0,
			"msg":    "",
			"data": map[string]interface{}{
				"token":    token,
				"redirect": redirect,
			},
		})
	}
}

// LogoutHandler 返回登出 API 的 Gin Handler。
//
// 浏览器页面请求（GET + Accept: text/html）：清除 Cookie 后 302 重定向到登录页。
// API 请求：返回 JSON。
func (c AuthConfig) LogoutHandler() gin.HandlerFunc {
	c.normalize()

	return func(ctx *gin.Context) {
		ctx.SetCookie(c.CookieName, "", -1, "/", "", false, true)

		// 浏览器页面请求 → 重定向到登录页
		if isPageRequest(ctx) {
			ctx.Redirect(http.StatusFound, c.LoginPath)
			return
		}

		// API 请求 → 返回 JSON
		ctx.JSON(http.StatusOK, gin.H{
			"status": 0,
			"msg":    c.Messages.LoggedOut,
			"data": map[string]interface{}{
				"redirect": c.LoginPath,
			},
		})
	}
}

// ============================================================
// SetupAuth — 一键鉴权集成
// ============================================================

// SetupAuth 一键注册登录页面和登录/登出 API 路由。
//
// 自动执行：
//  1. 在 Engine 上注册登录页面 Schema
//  2. 在 route 上挂载登录页面路由（GET LoginPath）
//  3. 在 route 上注册登录/登出 API 路由
//
// 注意：SetupAuth 不安装鉴权中间件。请在需要保护的路由组上手动调用 cfg.RequireAuth()。
func (e *Engine) SetupAuth(r gin.IRouter, cfg AuthConfig, auth Authenticator) {
	cfg.normalize()

	e.RegisterSchema(cfg.LoginPath, cfg.LoginSchema())
	r.GET(cfg.LoginPath, e.GinHandler(cfg.LoginPath))
	r.POST(cfg.LoginAPIPath, cfg.LoginHandler(auth))
	r.POST(cfg.LogoutAPIPath, cfg.LogoutHandler())
	r.GET(cfg.LogoutAPIPath, cfg.LogoutHandler())
}
