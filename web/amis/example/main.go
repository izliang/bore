// Example: amis + Gin 完整企业级管理后台（支持 i18n 国际化）
//
// 运行：go run ./web/amis/example/
//
// 路由：
//
//	/login               登录页面（公开）
//	/admin/dashboard     仪表盘（需登录）
//	/admin/users         用户管理 CRUD
//	/admin/models        模型管理 CRUD
//	/admin/settings      系统设置
//
// 测试账号：admin / admin123
//
// i18n：通过 ?lang=zh 或 ?lang=en 切换语言
package main

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/izliang/bore/i18n"
	"github.com/izliang/bore/web/amis"
	"github.com/izliang/bore/web/middleware"
)

// extraCSS 业务级样式定制，通过 amis.WithExtraCSS 注入。
// SDK 模板保持通用，业务方自由覆盖品牌色、导航、暗黑模式等。
const extraCSS = `
.cxd-Page-aside { border-right: 1px solid var(--borderColor, #e8e8e8) !important; }

html.dark {
    --amis-bg: #0f0f1a;
    --Page-aside-bg: #12122a;
    --Page-aside-width: 220px;
}
html.dark .cxd-Page { --Page-aside-bg: #12122a; }
html.dark .cxd-Page-aside {
    border-right-color: rgba(255,255,255,0.06) !important;
}

.cxd-Nav.cxd-Nav--stacked { padding: 8px 6px; }
.cxd-Nav.cxd-Nav--stacked .cxd-Nav-item {
    margin: 2px 6px;
    padding: 10px 14px;
    border-radius: 8px;
    transition: all 0.18s ease;
    font-size: 14px;
    color: var(--textColor, #334155);
    cursor: pointer;
}
.cxd-Nav.cxd-Nav--stacked .cxd-Nav-item.is-active {
    background: linear-gradient(135deg, rgba(99,102,241,0.12) 0%, rgba(139,92,246,0.10) 100%);
    color: #6366f1;
    font-weight: 600;
}
.cxd-Nav.cxd-Nav--stacked .cxd-Nav-item:hover:not(.is-active) {
    background: rgba(99,102,241,0.05);
    color: var(--textColor, #334155);
}
.cxd-Nav.cxd-Nav--stacked .cxd-Nav-itemIcon {
    margin-right: 10px;
    width: 20px;
    text-align: center;
    font-size: 15px;
    opacity: 0.7;
}
.cxd-Nav.cxd-Nav--stacked .cxd-Nav-item.is-active .cxd-Nav-itemIcon {
    opacity: 1;
    color: #6366f1;
}

html.dark .cxd-Nav.cxd-Nav--stacked .cxd-Nav-item {
    color: rgba(255,255,255,0.65);
}
html.dark .cxd-Nav.cxd-Nav--stacked .cxd-Nav-item.is-active {
    background: rgba(99,102,241,0.18);
    color: #a5b4fc;
}
html.dark .cxd-Nav.cxd-Nav--stacked .cxd-Nav-item:hover:not(.is-active) {
    background: rgba(255,255,255,0.05);
    color: rgba(255,255,255,0.85);
}

.shadow-sm { transition: box-shadow 0.25s ease, transform 0.25s ease; }
.shadow-sm:hover { box-shadow: 0 6px 24px rgba(0,0,0,0.09) !important; transform: translateY(-1px); }

.cxd-Page-toolbar {
    min-height: 52px;
    padding: 0 20px !important;
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 4px;
}

.cxd-Page-content { padding: 24px !important; }
.cxd-Page-body { padding: 0 !important; }

html.dark .cxd-Card { background: rgba(255,255,255,0.03); }
html.dark .cxd-Panel { background: rgba(255,255,255,0.02); }
html.dark .cxd-Table-table { background: transparent; }
html.dark .cxd-Table-tr:hover td { background: rgba(255,255,255,0.03); }
`

func main() {
	// ── 1. 加载 i18n 翻译包 ──────────────────────────────
	bundle := i18n.MustLoad(i18n.Config{
		Dir:         "web/amis/example/i18n/locales",
		DefaultLang: "zh",
		StrictMode:  true, // 启动时校验所有语言 key 一致性
	})

	// ── 2. 创建 amis Engine，注入 i18n ───────────────────
	engine := amis.New(
		amis.WithCDN("6.13.0", amis.ThemeCXD),
		amis.WithTitle("bore Admin"),
		amis.WithExtraCSS(extraCSS),
		amis.WithI18n(amis.I18nConfig{
			Bundle:     bundle,
			StrictMode: true, // Schema 注册时校验 $t.* key
		}),
	)

	// ── 3. 鉴权配置 ──────────────────────────────────────
	cfg := amis.DefaultAuthConfig()
	cfg.HomePath = "/admin/dashboard"

	authenticator := func(username, password string) (string, string, error) {
		if username == "admin" && password == "admin123" {
			return "session-token-admin", "", nil
		}
		return "", "", errors.New("invalid username or password")
	}

	r := gin.Default()

	// ── 4. 注册统一语言检测中间件 ─────────────────────────
	r.Use(middleware.LanguageMiddleware())

	// ── 5. 公开路由 ──────────────────────────────────────
	engine.SetupAuth(r, cfg, authenticator)
	engine.RegisterJSONFile("/login", "web/amis/example/pages/login.json")

	// ── 6. 受保护路由 ────────────────────────────────────
	protected := r.Group("/admin", cfg.RequireAuth())

	engine.RegisterJSONFile("/admin/dashboard", "web/amis/example/pages/dashboard.json")
	engine.RegisterJSONFile("/admin/users", "web/amis/example/pages/users.json")
	engine.RegisterJSONFile("/admin/models", "web/amis/example/pages/models.json")
	engine.RegisterJSONFile("/admin/settings", "web/amis/example/pages/settings.json")

	engine.BindGin(protected)

	// ── 7. 首页重定向 ────────────────────────────────────
	r.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/admin/dashboard")
	})

	// ====================================================
	// 模拟业务 API
	// ====================================================
	api := r.Group("/api")

	api.GET("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": "000000", "message": "success",
			"data": gin.H{
				"items": []gin.H{
					{"id": 1, "name": "Alice", "email": "alice@bore.io", "role": "Admin"},
					{"id": 2, "name": "Bob", "email": "bob@bore.io", "role": "Editor"},
					{"id": 3, "name": "Charlie", "email": "charlie@bore.io", "role": "Viewer"},
					{"id": 4, "name": "Diana", "email": "diana@bore.io", "role": "Editor"},
					{"id": 5, "name": "Eve", "email": "eve@bore.io", "role": "Viewer"},
				}, "total": 5,
			},
		})
	})
	api.POST("/users", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Created", "data": gin.H{"id": 99}})
	})
	api.PUT("/users/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Updated", "data": nil})
	})
	api.DELETE("/users/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Deleted", "data": nil})
	})

	api.GET("/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": 0, "msg": "",
			"data": gin.H{
				"items": []gin.H{
					{"id": 1, "name": "GPT-5.6-Sol", "version": "v3.0", "status": "active", "description": "Latest frontier coding model"},
					{"id": 2, "name": "GPT-5.6-Terra", "version": "v2.1", "status": "active", "description": "Balanced everyday coding model"},
					{"id": 3, "name": "GPT-5.6-Luna", "version": "v1.5", "status": "active", "description": "Fast and affordable coding model"},
					{"id": 4, "name": "GPT-5.5", "version": "v4.2", "status": "inactive", "description": "Previous generation frontier model"},
					{"id": 5, "name": "Claude-4.5-Opus", "version": "v2.0", "status": "active", "description": "Third-party flagship model"},
				}, "total": 5,
			},
		})
	})
	api.POST("/models", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Created", "data": gin.H{"id": 99}})
	})
	api.PUT("/models/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Updated", "data": nil})
	})
	api.DELETE("/models/:id", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Deleted", "data": nil})
	})

	api.GET("/activities", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"code": "000000", "message": "success",
			"data": gin.H{
				"items": []gin.H{
					{"time": "2026-07-28 19:30", "user": "Alice", "action": "Created model GPT-5.6-Sol", "status": "success"},
					{"time": "2026-07-28 18:15", "user": "Bob", "action": "Updated dataset v3", "status": "success"},
					{"time": "2026-07-28 17:42", "user": "admin", "action": "Deleted expired logs", "status": "success"},
					{"time": "2026-07-28 16:20", "user": "Charlie", "action": "Exported reports", "status": "failed"},
					{"time": "2026-07-28 15:08", "user": "Diana", "action": "Updated permission config", "status": "success"},
				},
			},
		})
	})

	api.GET("/system-info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": 0, "msg": "",
			"data": gin.H{
				"version": "v2.0.0", "goVersion": "go1.25.0",
				"uptime": "3d 12h", "memory": "256MB / 512MB",
			},
		})
	})

	api.PUT("/settings", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": 0, "msg": "Saved", "data": nil})
	})

	println("")
	println("╔══════════════════════════════════════════════╗")
	println("║  🔐 bore Admin  v2.0  (i18n)           ║")
	println("╠══════════════════════════════════════════════╣")
	println("║  http://localhost:8080                      ║")
	println("║                                              ║")
	println("║  /login             Login Page              ║")
	println("║  /admin/dashboard   Dashboard               ║")
	println("║  /admin/users       User Management         ║")
	println("║  /admin/models      Model Management        ║")
	println("║  /admin/settings    Settings                ║")
	println("║                                              ║")
	println("║  admin / admin123                            ║")
	println("║                                              ║")
	println("║  i18n: ?lang=en  (English)                  ║")
	println("║        ?lang=zh  (中文)                     ║")
	println("╚══════════════════════════════════════════════╝")
	println("")
	r.Run(":8080")
}
