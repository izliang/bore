# web/router/cyberhuma

`cyberhuma` 是 bore 基于 Gin + Huma v2 的泛型路由封装。它的核心目标是：

- 业务 Handler 继续使用 `define.TypedHandler[I, O]`：`func(context.Context, *I) (*O, e.ServiceError)`。
- 路由层自动生成 OpenAPI、做参数/JSON Body 校验、统一响应 Envelope。
- 支持挂载到 `*gin.Engine` 或 `*gin.RouterGroup`，并支持 API Group 的中间件和 OpenAPI 定制。

## 最小示例

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
    ID string `path:"id" doc:"用户 ID" example:"u_001"`
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
    api := cyberhuma.New(engine.Group("/api/v1"), cyberhuma.WithHumaDocs(router.DocConfig{
        Exposure: router.DocAlways,
        UI:       router.DocUISwagger,
    }))

    cyberhuma.GET(api, "/users/{id}", GetUser, cyberhuma.WithTags("用户"))

    _ = engine.Run(":8080")
}
```

运行时访问路径是 `/api/v1/users/u_001`。OpenAPI JSON 默认暴露在 `/api/v1/openapi.json`，文档页默认暴露在 `/api/v1/docs`。

## 路由路径与结构体标签速查

| 来源 | 路由写法 | 结构体标签 | 示例请求 | 说明 |
| --- | --- | --- | --- | --- |
| Path | `/users/{id}` | `path:"id"` | `/users/u_001` | Huma/OpenAPI 风格，不能写 Gin 的 `:id` |
| Query | `/users` | `query:"page"` | `/users?page=1` | 支持默认值、范围、枚举等校验标签 |
| Header | 任意 | `header:"X-Trace-ID"` | Header | 适合透传追踪、租户、版本等信息 |
| Cookie | 任意 | `cookie:"session_id"` | Cookie | 读取 cookie 参数 |
| JSON Body | `POSTBody` | `json:"name"` | JSON Body | 推荐用 `POSTBody` 注册标准 JSON 请求体 |
| 混合输入 | 任意 | `path/query/header` + `Body` | Path + Query + JSON | 需要同时读取参数和 Body 时使用 `Body` 字段 |

## Path 参数

### 正确写法

```go
type GetUserInput struct {
    ID string `path:"id" doc:"用户 ID" example:"u_001"`
}

cyberhuma.GET(api, "/users/{id}", GetUser)
```

请求：

```http
GET /users/u_001
```

### 易错点

```go
// 错误：cyberhuma 使用 Huma/OpenAPI 路径参数，不使用 Gin 的 :id。
cyberhuma.GET(api, "/users/:id", GetUser)

// 正确：
cyberhuma.GET(api, "/users/{id}", GetUser)
```

`path:"id"` 的名字必须和路由中的 `{id}` 一致。

```go
// 错误：路由是 {id}，结构体里写 path:"user_id"，无法正确绑定。
type BadInput struct {
    ID string `path:"user_id"`
}

// 正确：
type GoodInput struct {
    ID string `path:"id"`
}
```

## Query 参数

Query 参数使用 `query:"name"` 标签。

```go
type ListUsersInput struct {
    Page     int      `query:"page" default:"1" minimum:"1" doc:"页码，从 1 开始" example:"1"`
    PageSize int      `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"每页数量" example:"20"`
    Keyword  string   `query:"keyword" required:"false" maxLength:"50" doc:"搜索关键字"`
    Status   string   `query:"status" enum:"active,disabled" required:"false" doc:"用户状态"`
    Tags     []string `query:"tags" required:"false" doc:"标签，默认使用逗号分隔"`
}

cyberhuma.GET(api, "/users", ListUsers)
```

请求示例：

```http
GET /users?page=1&page_size=20&keyword=alice&status=active&tags=vip,internal
```

如果希望数组参数使用重复 query key，可以加 `explode`：

```go
type ListUsersInput struct {
    Tags []string `query:"tags,explode" required:"false" doc:"标签，重复 query key 形式"`
}
```

请求示例：

```http
GET /users?tags=vip&tags=internal
```

### Query 必填与可选

Huma 对参数字段支持 `required` 标签。建议对可选 query 显式写 `required:"false"`，尤其是字符串、数字、数组等零值容易产生歧义的字段。

```go
type SearchInput struct {
    // 必填：请求中必须提供 q。
    Q string `query:"q" required:"true" minLength:"1"`

    // 可选：不提供时使用默认值 1。
    Page int `query:"page" required:"false" default:"1" minimum:"1"`
}
```

> 注意：Huma 不支持 path/query/header/form/cookie 参数字段使用指针类型。不要写 `*string`、`*int` 作为 query/path 字段；需要可选语义时使用 `required:"false"`、`default` 或零值判断。

## JSON Body：推荐使用 POSTBody

对于标准 JSON 请求体，推荐使用 `POSTBody[I, O]`。此时 `I` 就是业务 JSON 对象，字段使用 `json` 标签。

```go
type CreateUserInput struct {
    Name  string   `json:"name" doc:"姓名" minLength:"1" maxLength:"50" example:"alice"`
    Age   int      `json:"age,omitempty" minimum:"0" maximum:"150" doc:"年龄" example:"18"`
    Email string   `json:"email" format:"email" doc:"邮箱" example:"alice@example.com"`
    Roles []string `json:"roles,omitempty" doc:"角色列表" example:"admin,user"`
}

type CreateUserOutput struct {
    ID string `json:"id"`
}

func CreateUser(ctx context.Context, input *CreateUserInput) (*CreateUserOutput, e.ServiceError) {
    return &CreateUserOutput{ID: "u_001"}, nil
}

cyberhuma.POSTBody(api, "/users", CreateUser, cyberhuma.WithTags("用户"))
```

请求示例：

```http
POST /users
Content-Type: application/json

{
  "name": "alice",
  "age": 18,
  "email": "alice@example.com",
  "roles": ["admin", "user"]
}
```

### JSON 字段必填规则

Huma 生成 schema 时，普通 JSON 字段默认倾向于必填；带 `omitempty` / `omitzero` 或 `required:"false"` 的字段会被视为可选。

```go
type CreateInput struct {
    // 必填：没有 omitempty，也没有 required:"false"。
    Name string `json:"name" minLength:"1"`

    // 可选：omitempty。
    Nickname string `json:"nickname,omitempty" maxLength:"50"`

    // 可选：显式 required:false。
    Remark string `json:"remark" required:"false" maxLength:"200"`

    // 可选且允许 null：指针 + omitempty。
    AvatarURL *string `json:"avatar_url,omitempty" format:"uri"`
}
```

建议：

- 必填字段：不要加 `omitempty`，并加上明确校验，如 `minLength:"1"`。
- 可选字段：加 `omitempty` 或 `required:"false"`。
- 需要区分“不传”和“传空值/传 null”：使用指针字段，例如 `*string`，并配合 `json:",omitempty"`。

## 混合输入：Path/Query/Header + Body

如果一个接口既需要 path/query/header，又需要 JSON Body，不要用 `POSTBody`，而是直接定义一个包含 `Body` 字段的输入结构体，并使用 `POST` 或 `PUT`。

```go
type UpdateUserBody struct {
    Name  string `json:"name" minLength:"1" maxLength:"50" doc:"姓名"`
    Email string `json:"email" format:"email" doc:"邮箱"`
}

type UpdateUserInput struct {
    ID      string         `path:"id" doc:"用户 ID" example:"u_001"`
    DryRun  bool           `query:"dry_run" required:"false" default:"false" doc:"是否仅校验不落库"`
    TraceID string         `header:"X-Trace-ID" required:"false" doc:"链路追踪 ID"`
    Body    UpdateUserBody `json:"body" doc:"请求体"`
}

func UpdateUser(ctx context.Context, input *UpdateUserInput) (*UserOutput, e.ServiceError) {
    // input.ID      来自 path
    // input.DryRun  来自 query
    // input.TraceID 来自 header
    // input.Body    来自 JSON body
    return &UserOutput{ID: input.ID, Name: input.Body.Name}, nil
}

cyberhuma.PUT(api, "/users/{id}", UpdateUser)
```

请求示例：

```http
PUT /users/u_001?dry_run=false
X-Trace-ID: trace-001
Content-Type: application/json

{
  "name": "alice",
  "email": "alice@example.com"
}
```

> 说明：Huma 识别名为 `Body` 的字段作为请求体。`Body` 字段里的结构体使用 `json` 标签描述 JSON 字段名和校验规则。

## GET / DELETE 通常只放参数，不放 Body

GET 和 DELETE 推荐只使用 path/query/header/cookie 参数：

```go
type DeleteUserInput struct {
    ID     string `path:"id" doc:"用户 ID"`
    Reason string `query:"reason" required:"false" maxLength:"200" doc:"删除原因"`
}

cyberhuma.DELETE(api, "/users/{id}", DeleteUser)
```

请求示例：

```http
DELETE /users/u_001?reason=duplicate
```

## 输出结构体

业务 Handler 返回的 `O` 会被包进统一响应信封的 `data` 字段中。输出结构体使用标准 `json` 标签。

```go
type UserOutput struct {
    ID        string   `json:"id" doc:"用户 ID" example:"u_001"`
    Name      string   `json:"name" doc:"姓名" example:"alice"`
    Email     string   `json:"email,omitempty" doc:"邮箱" example:"alice@example.com"`
    Roles     []string `json:"roles,omitempty" doc:"角色"`
    CreatedAt string   `json:"created_at" format:"date-time" doc:"创建时间"`
}
```

实际响应示例：

```json
{
  "status": 200,
  "body": {
    "code": "000000",
    "message": "success",
    "data": {
      "id": "u_001",
      "name": "alice",
      "email": "alice@example.com",
      "roles": ["admin"],
      "created_at": "2026-05-30T10:00:00Z"
    },
    "request_id": "req-001"
  }
}
```

## 常用校验/文档标签

这些标签由 Huma 读取，用于 OpenAPI schema 和运行时校验。

| 标签 | 示例 | 说明 |
| --- | --- | --- |
| `doc` | `doc:"用户 ID"` | 字段说明 |
| `example` | `example:"u_001"` | 示例值 |
| `default` | `default:"1"` | 默认值，常用于 query |
| `required` | `required:"false"` | 覆盖必填判断 |
| `enum` | `enum:"active,disabled"` | 枚举值 |
| `minimum` / `maximum` | `minimum:"1" maximum:"100"` | 数值范围 |
| `minLength` / `maxLength` | `minLength:"1" maxLength:"50"` | 字符串长度 |
| `pattern` | `pattern:"^[a-z0-9_]+$"` | 正则校验 |
| `format` | `format:"email"` | OpenAPI format，如 `email`、`uri`、`date-time` |
| `minItems` / `maxItems` | `minItems:"1" maxItems:"10"` | 数组长度 |
| `uniqueItems` | `uniqueItems:"true"` | 数组元素唯一 |
| `nullable` | `nullable:"true"` | 标记可空，适合标量类型 |
| `hidden` | `hidden:"true"` | 不展示在 OpenAPI 文档中 |

## 注册函数选择

| 函数 | 典型输入结构 | 适合场景 |
| --- | --- | --- |
| `GET[I, O]` | `path/query/header/cookie` 字段 | 查询接口 |
| `DELETE[I, O]` | `path/query/header/cookie` 字段 | 删除接口 |
| `POSTBody[I, O]` | 纯 JSON Body 结构体 | 创建接口，推荐用于标准 JSON 请求体 |
| `POST[I, O]` | 参数字段 + `Body` 字段，或高级 Huma 输入 | 需要同时读取 path/query/header 和 Body |
| `PUT[I, O]` | 参数字段 + `Body` 字段 | 更新接口 |

## API Group 示例

```go
api := cyberhuma.New(engine.Group("/api/v1"))

admin := api.Group("/admin",
    cyberhuma.WithGroupGinMiddleware(AdminAuth()),
    cyberhuma.WithGroupSimpleOperationModifier(func(o *huma.Operation) {
        o.Tags = append(o.Tags, "admin")
    }),
)

cyberhuma.GET(admin, "/users/{id}", GetAdminUser)
cyberhuma.POSTBody(admin, "/users", CreateAdminUser)
```

如果根路由是 `engine.Group("/api/v1")`，则：

- 运行时路径：`/api/v1/admin/users/{id}`
- OpenAPI path：`/admin/users/{id}`

如果希望 OpenAPI 显示 API 根地址，使用：

```go
api := cyberhuma.New(engine.Group("/api/v1"), cyberhuma.WithHumaConfig(func(c *huma.Config) {
    c.OpenAPI.Servers = []*huma.Server{{URL: "/api/v1"}}
}))
```

## 最容易混淆的点

1. `cyberhuma` 的 path 写法是 `/users/{id}`，不是 Gin 的 `/users/:id`。
2. `path:"id"` / `query:"page"` / `header:"X-Trace-ID"` 这些字段是请求参数，不是 JSON Body。
3. JSON Body 字段必须使用 `json:"..."`，推荐用 `POSTBody` 表达纯 Body 输入。
4. 同时需要参数和 Body 时，输入结构体中必须有名为 `Body` 的字段，并用 `POST`/`PUT` 注册。
5. Path/Query/Header/Cookie 参数字段不要用指针类型；JSON Body 内部字段可以使用指针表达可选/null 语义。
6. Query 数组默认适合逗号分隔；需要重复 key 时使用 `query:"name,explode"`。
7. 默认不暴露文档；需要 Swagger/Redoc 时必须显式传 `WithHumaDocs`。
