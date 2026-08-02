# web/router

`web/router` 是 bore 对 Gin 路由的统一封装层，目标是让业务接口保持同一套 `define.TypedHandler[I, O]` 写法，同时按场景选择轻量 Gin 路由或带 OpenAPI/参数校验的 Huma 路由。

## 包结构

| 路径 | 作用 | 适用场景 |
| --- | --- | --- |
| `router.go` | 共享类型和工具：`MetricsReporter`、`HTTPStatusFromError`、`APINameFromPath` | 子包复用的基础能力 |
| `doc.go` | OpenAPI 文档暴露策略：`DocConfig`、Swagger/Redoc UI 注册 | Huma 路由文档开关 |
| `simple/` | 纯 Gin 泛型路由封装 | 内部服务、无需 OpenAPI、追求最小依赖和开销 |
| `cyberhuma/` | Gin + Huma v2 泛型路由封装 | 对外 API、需要 OpenAPI、自动参数校验、API Group 定制 |
| `huma/` | 旧版 Huma 封装 | 已废弃，请使用 `cyberhuma/` |
| `swagger.html` / `redoc.html` | 内嵌文档 UI 模板 | 编译期嵌入，无需运行时文件 |

## 核心概念

### 统一 Handler 签名

`simple` 和 `cyberhuma` 都面向同一类业务 Handler：

```go
func(ctx context.Context, input *Input) (*Output, e.ServiceError)
```

这意味着业务逻辑可以在两种路由模式之间迁移，而不需要改 Handler 本身。

### 统一响应格式

路由层会把成功和错误结果包装成 `define.Envelope[T]`：

```json
{
  "code": "000000",
  "message": "success",
  "data": {}
}
```

业务错误通过 `e.ServiceError` 返回，HTTP 状态码由 `router.HTTPStatusFromError` 决定：

- 鉴权错误：`403`
- 资源不存在：`404`
- 客户端错误：`400`
- 其他业务错误：`200`，错误码由响应体表达

## simple：纯 Gin 路由

`simple` 适合内部接口或不需要 OpenAPI 的服务。

```go
package main

import (
    "context"

    "github.com/izliang/bore/e"
    "github.com/izliang/bore/web/router/simple"
    "github.com/gin-gonic/gin"
)

type CreateUserInput struct {
    Name string `json:"name" binding:"required"`
}

type UserOutput struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

func CreateUser(ctx context.Context, input *CreateUserInput) (*UserOutput, e.ServiceError) {
    return &UserOutput{ID: "1", Name: input.Name}, nil
}

func main() {
    engine := gin.New()
    api := simple.New(engine.Group("/api/v1"))

    simple.POST(api, "/users", CreateUser)

    _ = engine.Run(":8080")
}
```

### simple 能力

- `GET/POST/PUT/DELETE` 泛型注册
- Gin `ShouldBindJSON` / `ShouldBindQuery` / `ShouldBindUri` 自动绑定
- 统一 Envelope 响应
- 可选 i18n 翻译：`simple.WithTranslator(...)`
- 可选 Metrics：`simple.WithMetrics(...)`
- 直接复用 Gin Group 和 Gin Middleware

## cyberhuma：OpenAPI + 自动校验路由

`cyberhuma` 适合需要 OpenAPI 文档、参数 schema、自动校验和前后端协作的 API。更完整的 path/query/body/json 结构体标签示例见 [`cyberhuma/README.md`](cyberhuma/README.md)。

```go
package main

import (
    "context"

    "github.com/izliang/bore/e"
    "github.com/izliang/bore/web/router"
    "github.com/izliang/bore/web/router/cyberhuma"
    "github.com/gin-gonic/gin"
)

type GetUserInput struct {
    ID string `path:"id" doc:"用户 ID" example:"1"`
}

type UserOutput struct {
    ID   string `json:"id"`
    Name string `json:"name"`
}

func GetUser(ctx context.Context, input *GetUserInput) (*UserOutput, e.ServiceError) {
    return &UserOutput{ID: input.ID, Name: "alice"}, nil
}

func main() {
    engine := gin.New()

    api := cyberhuma.New(
        engine.Group("/api/v1"),
        cyberhuma.WithHumaTitle("User API", "1.0.0"),
        cyberhuma.WithHumaDocs(router.DocConfig{
            Exposure: router.DocAlways,
            UI:       router.DocUISwagger,
        }),
        cyberhuma.WithHumaConfig(func(c *huma.Config) {
            c.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1"}}
        }),
    )

    cyberhuma.GET(api, "/users/{id}", GetUser, cyberhuma.WithTags("用户"))

    _ = engine.Run(":8080")
}
```

> 注意：上例中的 `WithHumaConfig` 需要额外 import `github.com/danielgtaylor/huma/v2`。

### cyberhuma 能力

- `GET/POST/POSTBody/PUT/DELETE` 泛型注册
- 兼容 `simple` 的业务 Handler 签名
- Huma v2 自动 OpenAPI schema 生成
- Huma v2 参数和 Body 校验
- 可配置 Swagger / Redoc / 仅 OpenAPI JSON
- 支持直接挂载到 `*gin.Engine` 或 `*gin.RouterGroup`
- 支持 API Group：按组增加 Gin Middleware、Huma Middleware、Transformer、Operation Modifier

## API Group 和中间件

### 挂载到已有 Gin Group

当一个服务有统一前缀或全局中间件时，可以先创建 Gin Group，再交给 `cyberhuma.New`：

```go
apiGroup := engine.Group("/api/v1",
    RequestIDMiddleware(),
    CORSMiddleware(),
)

api := cyberhuma.New(apiGroup)
cyberhuma.GET(api, "/ping", Ping)
```

运行时路由是：

- `GET /api/v1/ping`

OpenAPI path 默认保持相对 API 根路径：

- `/ping`

如果希望 OpenAPI 明确展示 API 根地址，使用 Huma 原生配置：

```go
api := cyberhuma.New(apiGroup, cyberhuma.WithHumaConfig(func(c *huma.Config) {
    c.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1"}}
}))
```

### 创建 cyberhuma 子 Group

对部分接口增加定制能力时，使用 `api.Group(...)`：

```go
admin := api.Group("/admin",
    cyberhuma.WithGroupGinMiddleware(AdminAuthMiddleware()),
    cyberhuma.WithGroupSimpleOperationModifier(func(o *huma.Operation) {
        o.Tags = append(o.Tags, "admin")
        o.Security = []map[string][]string{{"bearerAuth": {}}}
    }),
)

cyberhuma.GET(admin, "/users", ListAdminUsers)
cyberhuma.POSTBody(admin, "/users", CreateAdminUser)
```

运行时路由：

- `GET /admin/users`
- `POST /admin/users`

OpenAPI path：

- `/admin/users`

如果根路由是 `engine.Group("/api/v1")`，运行时路由会变成 `/api/v1/admin/users`，OpenAPI path 仍然相对 API 根路径显示为 `/admin/users`。

### GroupOption 列表

| Option | 作用 |
| --- | --- |
| `WithGroupGinMiddleware(...gin.HandlerFunc)` | 给当前组增加 Gin 中间件，例如鉴权、限流、审计 |
| `WithGroupHumaMiddleware(...func(huma.Context, func(huma.Context)))` | 给当前组增加 Huma 中间件 |
| `WithGroupTransformers(...huma.Transformer)` | 给当前组增加响应转换器 |
| `WithGroupOperationModifier(func(*huma.Operation, func(*huma.Operation)))` | 给当前组增加完整 Operation 修改器 |
| `WithGroupSimpleOperationModifier(func(*huma.Operation))` | 给当前组增加简单 Operation 修改器，常用于 tags/security/summary |

## 文档暴露策略

`router.DocConfig` 控制文档是否注册：

```go
cyberhuma.New(engine, cyberhuma.WithHumaDocs(router.DocConfig{
    Exposure: router.DocAlways, // 开发/测试环境
    UI:       router.DocUISwagger,
}))
```

可选值：

| Exposure | 行为 |
| --- | --- |
| `DocNever` | 默认值，不暴露文档，生产安全 |
| `DocAlways` | 始终暴露文档，适合开发/测试 |
| `DocConditional` | 通过 `Condition` 动态判断是否暴露 |

UI 类型：

| UI | 行为 |
| --- | --- |
| `DocUISwagger` | 注册 Swagger UI |
| `DocUIRedoc` | 注册 Redoc UI |
| `DocUINone` | 只暴露 OpenAPI JSON |

默认路径：

- OpenAPI JSON：`/openapi.json`
- 文档页面：`/docs`

如果 `cyberhuma.New` 挂载在 Gin Group 上，文档路径也跟随 Gin Group 前缀。例如 `engine.Group("/api/v1")` 下会暴露：

- `/api/v1/openapi.json`
- `/api/v1/docs`

## 路由模式选择建议

| 场景 | 推荐 |
| --- | --- |
| 内部服务、无文档要求、追求轻量 | `simple` |
| 对外 API、需要 Swagger/Redoc、参数校验 | `cyberhuma` |
| 需要按业务域拆分鉴权/中间件 | `cyberhuma.New(engine.Group(...))` 或 `api.Group(...)` |
| 旧代码仍引用 `web/router/huma` | 逐步迁移到 `web/router/cyberhuma` |

## 常见注意事项

1. `cyberhuma` 路径参数使用 Huma/OpenAPI 风格：`/users/{id}`，不要写成 Gin 的 `/users/:id`。
2. `simple` 路径参数使用 Gin 风格：`/users/:id`。
3. `cyberhuma.POSTBody` 会把业务输入包在 `define.BodyWrapper[I]` 中，更适合标准 JSON Body 输入。
4. 默认不暴露文档；需要文档时必须显式传入 `WithHumaDocs`。
5. API 根路径通过 Gin Group 控制；OpenAPI 根路径建议通过 `WithHumaConfig` 的 `OpenAPI.Servers` 表达。
