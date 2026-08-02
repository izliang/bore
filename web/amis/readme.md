# amis — 百度 amis + Gin 无缝集成 SDK

后端开发者只需配置 JSON Schema 或指定 JSON 文件路径，
即可自动映射 Gin 路由并渲染 amis 管理后台，无需编写任何前端代码。

---

## 快速开始

```go
import "github.com/izliang/bore/web/amis"

engine := amis.New(amis.WithCDN("6.10.0", amis.ThemeCXD))
engine.LoadPagesDir("/admin", "./pages")

r := gin.Default()
engine.BindGin(r.Group("/admin"))
r.Run(":8080")
```

运行示例：

```bash
go run ./web/amis/example/
# → http://localhost:8080
# 测试账号: admin / admin123
```

---

## 核心接口

### AssetProvider — 静态资源版本解耦

```go
type AssetProvider interface {
    Version() string
    CSSURLs() []string
    JSURLs()  []string
}
```

**内置实现：`CDNAssets`**（基于 unpkg CDN）

```go
// 指定版本 + 主题
amis.NewCDNAssets("6.10.0", amis.ThemeCXD)

// latest 版本 + 暗黑主题
amis.NewCDNAssets("", amis.ThemeDark)

// 自定义 CDN 基地址（内网 npm 镜像）
amis.NewCDNAssets("6.10.0", amis.ThemeCXD).WithBaseURL("https://npm.internal.com")
```

### Engine — 页面注册中心

| 方法 | 说明 |
|---|---|
| `RegisterSchema(path, schema)` | 注册 JSON 字符串 / Map / Struct |
| `RegisterJSONFile(path, file)` | 加载单个 JSON 文件 |
| `LoadPagesDir(prefix, dir)` | 批量扫描目录（`.json` → 路由） |
| `Render(path)` | 渲染为完整 HTML |
| `Pages()` | 返回所有已注册路径 |

### Gin 适配

| 方法 | 说明 |
|---|---|
| `GinHandler(path)` | 返回 `gin.HandlerFunc`，可单独挂载 |
| `BindGin(router)` | 一键挂载所有页面，自动剥离路由组前缀 |
| `SetupAuth(router, cfg, auth)` | 一键注册登录页面 + API |

---

## 内置鉴权

```go
cfg := amis.DefaultAuthConfig()
cfg.HomePath = "/admin"

engine.SetupAuth(r, cfg, func(username, password string) (string, string, error) {
    if username == "admin" && password == "pass" {
        return "token-xxx", "", nil
    }
    return "", "", errors.New("用户名或密码错误")
})

// 用自定义 JSON 覆盖默认登录页
engine.RegisterJSONFile("/login", "pages/login.json")

// 受保护路由组
protected := r.Group("/admin", cfg.RequireAuth())
engine.BindGin(protected)
```

**鉴权流程：**

```
浏览器访问 /admin/dashboard（无 Cookie）
  → 302 → /login?redirect=%2Fadmin%2Fdashboard

POST /api/login { username, password }
  → 200 + Set-Cookie: amis_token=xxx; HttpOnly

浏览器再次访问 /admin/dashboard（带 Cookie）
  → 200 OK
```

**AuthConfig 可配置项：**

| 字段 | 默认值 | 说明 |
|---|---|---|
| `LoginPath` | `/login` | 登录页面路径 |
| `LoginAPIPath` | `/api/login` | 登录 API 端点 |
| `LogoutAPIPath` | `/api/logout` | 登出 API 端点 |
| `HomePath` | `/` | 登录后默认跳转 |
| `CookieName` | `amis_token` | Cookie 名称 |
| `TokenLookup` | Cookie 读取 | 自定义 Token 提取（Header/URL 参数） |
| `Skipper` | 自动跳过登录路径 | 自定义跳过鉴权的路径 |

---

## 响应格式自动适配

模板 `fetcher` 内置 `adaptAmisResponse` 函数，自动将 bore 后端响应格式转换为 amis 兼容格式：

| 源格式 | 示例 | → amis |
|---|---|---|
| **HumaResponse** | `{"status":200,"body":{"code":"000000","message":"success","data":{...}}}` | `{"status":0,"msg":"success","data":{...}}` |
| **Envelope** | `{"code":"000000","message":"success","data":{...}}` | `{"status":0,"msg":"success","data":{...}}` |
| **Amis 原生** | `{"status":0,"msg":"","data":{...}}` | 透传 |

无需任何后端代码修改。

---

## 模板引擎

**默认模板**（`go:embed` 编译时注入，零外部文件依赖）：

- 加载 amis SDK JS/CSS
- 注入页面 Schema JSON
- 内置 `fetch` 桥接器 + 响应格式适配
- 支持暗黑模式 CSS 变量

**自定义模板**：

```go
amis.New(amis.WithCustomTemplate(myHTML))
```

模板占位符：`{{.Title}}` `{{.CSSURLs}}` `{{.JSURLs}}` `{{.SchemaJSON}}` `{{.Extra.key}}`

---

## Options 一览

| Option | 说明 |
|---|---|
| `WithCDN(version, theme)` | 快捷设置 CDN 资源 |
| `WithAssets(provider)` | 自定义 AssetProvider |
| `WithTitle(title)` | 页面默认标题 |
| `WithCustomTemplate(content)` | 注入自定义 HTML 模板 |
| `WithExtraData(data)` | 模板渲染扩展数据 |

---

## 文件结构

```
web/amis/
├── doc.go            # 包声明
├── assets.go         # AssetProvider 接口 + CDNAssets
├── auth.go           # AuthConfig + RequireAuth + LoginHandler + SetupAuth
├── engine.go         # Engine 核心（注册/渲染/LoadPagesDir）
├── gin.go            # GinHandler + BindGin（前缀自动剥离）
├── options.go        # 功能选项
├── template.go       # PageContext + 模板解析
├── template.html     # 默认 HTML 模板（go:embed）
├── readme.md
└── example/
    ├── main.go
    └── pages/
        ├── login.json    # 精美登录页（渐变背景）
```

---

## amis API 响应规范

amis 前端期望后端 API 返回统一格式：

```json
{
  "status": 0,
  "msg": "",
  "data": { "items": [...], "total": 10 }
}
```

- `status: 0` 表示成功，非 0 表示业务错误
- CRUD 组件从 `data.items` / `data.total` 读取列表数据
