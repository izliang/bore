# Config

类型安全的配置容器，支持热加载、Go 模板渲染和多种配置源。

## 快速开始

```go
package main

import (
    "github.com/izliang/bore/config"
    "github.com/izliang/bore/config/source"
)

type AppConfig struct {
    Service  ServiceConfig  `yaml:"service"`
    Database DatabaseConfig `yaml:"database"`
}

type ServiceConfig struct {
    Name string `yaml:"name"`
    Port int    `yaml:"port"`
}

type DatabaseConfig struct {
    DSN string `yaml:"dsn"`
}

func main() {
    cfg := config.New[AppConfig]()

    // 方式一：从项目根目录加载 YAML 文件
    cfg.LoadRootFile("conf/config.yaml")

    // 方式二：使用 Viper 源（YAML + 环境变量覆盖）
    cfg.SetTemplates(map[string]string{"Env": "prod"})
    cfg.Load(source.Viper("conf/config.yaml", "APP"))

    // 方式三：纯环境变量
    cfg.Load(source.Env("MYAPP_", "_"))

    // 类型安全读取
    port := cfg.Get().Service.Port
}
```

## 核心概念

### Cfg[T]

泛型配置容器，`T` 是用户定义的配置结构体。

| 方法 | 说明 |
|------|------|
| `New[T]()` | 创建容器 |
| `SetTemplates(map[string]string)` | 注册模板变量，在 Load/Reload 时自动应用 |
| `Load(source)` | 从 Source 加载配置 |
| `Reload()` | 重新读取（使用上次的 Source + 模板） |
| `Get() T` | 线程安全获取当前配置副本 |
| `Watch(func(old, new T))` | 注册变更回调，返回取消函数 |
| `Loaded() bool` | 是否已成功加载 |
| `LoadRootFile(relPath)` | 从项目根目录加载 YAML |

### Source 接口

```go
type Source interface {
    Load() ([]byte, error)
}
```

所有 Source 实现统一在 `config/source` 子包：

| 构造函数 | 说明 |
|----------|------|
| `source.File(relPath)` | 读取项目根目录下的 YAML 文件 |
| `source.Viper(relPath, prefix)` | viper 驱动：YAML 文件 + 环境变量叠加，支持类型强制转换 |
| `source.Env(prefix, delim)` | 纯环境变量，构建 YAML 文档 |

### 模板渲染

`SetTemplates` 注册 Go `text/template` 变量，对配置原始字节做 `{{.Var}}` 替换。模板在 Cfg 层统一处理，对所有 Source 实现生效。

```go
cfg := config.New[AppConfig]()
cfg.SetTemplates(map[string]string{
    "Env":  "prod",
    "Host": "db.example.com",
})
cfg.Load(source.File("conf/config.yaml"))
```

YAML 文件中可以使用占位符：
```yaml
service:
  name: myapp-{{.Env}}
database:
  dsn: "user:pass@tcp({{.Host}}:3306)/db"
```

模板在 Reload 时同样生效，变量持久化在 Cfg 实例中。

## Viper 源

`source.Viper` 基于 [spf13/viper](https://github.com/spf13/viper)，提供：

- **YAML 文件读取** — 路径相对于项目根目录
- **环境变量覆盖** — 通过 `AutomaticEnv` 叠加到文件配置之上
- **类型强制转换** — 环境变量字符串值自动匹配 YAML 中的类型（`"9090"` → `int 9090`，`"true"` → `bool true`）
- **深层嵌套支持** — `APP_A_B_C=val` 映射到 `a.b.c`

```go
cfg.Load(source.Viper("conf/config.yaml", "APP"))

// 环境变量：
//   APP_SERVICE_NAME=override  →  覆盖 service.name
//   APP_SERVICE_PORT=9090      →  覆盖 service.port（int）
//   APP_DATABASE_DSN=...       →  覆盖 database.dsn
```

### 环境变量命名规则

- 前缀（如 `APP`）后自动加 `_`
- 嵌套键用 `_` 分隔：`database.dsn` → `APP_DATABASE_DSN`
- 前缀末尾的 `_` 会被自动去除

### 已知限制

Viper 的 `AutomaticEnv` 只在配置文件中已声明的 key 上生效。纯环境变量（key 不在 YAML 中）不会被自动发现。如需 env-only 模式，请使用 `source.Env`。

## 项目根目录

`GetProjectRootDir()` 和 `LoadRootFile()` 按以下策略逐层向上查找项目根目录：

1. 从可执行文件目录向上，查找 `.root` 或 `go.mod`
2. 从工作目录向上，查找 `.root` 或 `go.mod`
3. 回退到可执行文件目录

## 测试

```bash
go test ./config/... -v
```
