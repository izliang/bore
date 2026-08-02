# i18n — 国际化翻译包

零外部依赖的 Go 国际化库，兼容 go-i18n v2 文件格式。仅用标准库 `encoding/json` + `text/template`。

---

## 快速开始

```go
package main

import "github.com/izliang/bore/i18n"

var I18N = i18n.MustLoad(i18n.Config{
    Dir:         "./locales",
    DefaultLang: "zh",
    StrictMode:  true,
})

func main() {
    msg := I18N.T("zh", "greeting")  // "你好"
}
```

---

## 设计理念

- **零外部依赖**：仅用 `encoding/json` + `text/template`
- **格式兼容**：自动识别 3 种 JSON 格式（go-i18n 扁平/数组 + 新数组），无需人工指定
- **语言前缀**：文件名 `zh_errors.json` 自动归并到 `"zh"`，同语言多文件自动合并
- **Key 校验**：启动时并集基准校验所有语言的 key 一致性
- **线程安全**：所有方法可并发调用
- **中文注释**：全部源码使用中文注释

---

## 目录结构 & 语言识别

```
locales/
├── zh.json            # 中文核心翻译（go-i18n 扁平格式，存量）
├── zh_errors.json     # 中文错误消息（新数组格式，自动合并到 zh）
├── en.json            # 英文翻译
└── ja.json            # 日文翻译（新数组格式）
```

**语言识别规则**：文件名以下划线 `_` 分隔，第一个段（前缀）即为语言代码。无下划线时，整个文件名（去掉 `.json`）为语言代码。

```
zh.json           → lang="zh"
zh_common.json    → lang="zh"
zh_errors.json    → lang="zh"
en.json           → lang="en"
en_ui.json        → lang="en"
```

**同语言多文件自动合并**：所有同前缀文件加载后合并为一组翻译。同 key 冲突时，按文件名排序，后加载的覆盖先加载的。翻译可按模块/域拆分：`zh_ui.json`、`zh_errors.json`、`zh_email.json`。

**隐藏文件过滤**：以 `.` 开头的文件（如 macOS 的 `._zh.json`、`.DS_Store`）自动忽略。

---

## 翻译文件格式

SDK 支持三种 JSON 格式，可混在同一目录，逐文件自动检测。

### 格式 1：新数组格式（推荐）

```json
[
  {"key": "greeting",        "value": "你好",                      "desc": "问候语"},
  {"key": "farewell",        "value": "再见 {{.Name}}",            "desc": "告别语"},
  {"key": "error.not_found", "value": "未找到资源: {{.ID}}"},
  {"key": "btn.submit",      "value": "提交",                      "tags": ["button"]}
]
```

检测特征：JSON 顶层为数组 + 首个元素含 `"key"` 字段。

**为什么新格式优于 go-i18n 格式：**

| 维度 | 新格式 `[{"key":"k","value":"v"}]` | go-i18n 扁平 `{"k":{"other":"v"}}` |
|------|---|---|
| 字段语义 | `key`/`value` 含义不言自明 | `other` 是 ICU 复数术语，非 i18n 背景的人困惑 |
| 嵌套层级 | 1 层（数组内对象） | 2 层（key → {other → value}） |
| 扩展字段 | 直接在对象上加 `"desc"`/`"tags"`/任意字段 | 扩展字段与 `other` 同层级，关系不清晰 |
| 翻译人员体验 | 阅读 `key`+`value` 一目了然 | 阅读 `other` 需要额外解释 |

新格式使用对象数组而非 map，因为数组支持扩展字段、保证顺序、易于转为 CSV/Excel 交付翻译。

### 格式 2：go-i18n v2 扁平（存量兼容）

```json
{
  "greeting":       {"other": "你好",                "description": "问候语"},
  "farewell":       {"other": "再见 {{.Name}}"},
  "error.not_found": {"other": "未找到资源: {{.ID}}"}
}
```

检测特征：JSON 顶层为对象 + 成员值含 `"other"` 字段。

### 格式 3：go-i18n v2 数组（存量兼容）

```json
[
  {"id": "greeting",  "other": "你好",                "description": "问候语"},
  {"id": "farewell",  "other": "再见 {{.Name}}"},
  {"id": "btn.submit", "other": "提交"}
]
```

检测特征：JSON 顶层为数组 + 首个元素含 `"id"` 字段（且不含 `"key"`）。

### 字段映射表

三种格式解析后统一归一化为 `Entry` 结构体：

| Entry 字段    | 格式 1（新数组）   | 格式 2（go-i18n 扁平）                            | 格式 3（go-i18n 数组） |
|--------------|-------------------|-------------------------------------------------|----------------------|
| `Key`        | `"key"`           | JSON 对象的 key 名                                | `"id"`               |
| `Value`      | `"value"`         | `"other"`                                        | `"other"`            |
| `Description`| `"desc"`          | `"description"`                                  | `"description"`      |
| `Deprecated` | `"deprecated"`    | 无，需手动在 Extra 取                              | 同左                   |
| `Tags`       | `"tags"`          | 无，需手动在 Extra 取                              | 同左                   |
| `Extra`      | 其余所有字段        | `"other"`/`"description"` 之外的所有字段（含复数形式） | 同左                   |

**go-i18n 复数处理**：`"zero"`、`"one"`、`"two"`、`"few"`、`"many"` 均收入 `Entry.Extra`。`"other"` 映射为 `Entry.Value`。SDK 不实现复数选择逻辑，需要复数的消费者自行从 Extra 取值。

### 自动检测逻辑

```
对于每个 *.json 文件：
  1. 尝试 json.Unmarshal 到 []json.RawMessage
     ├── 成功 + 首个元素有 "key" 字段 → 格式 1（新数组）
     └── 成功 + 首个元素有 "id" 字段  → 格式 3（go-i18n 数组）

  2. 尝试 json.Unmarshal 到 map[string]json.RawMessage
     └── 成功 + 值含 "other" 字段     → 格式 2（go-i18n 扁平）

  3. 都不匹配 → 返回错误 "unrecognized format"
```

三种格式的模板语法均为 Go `{{.Var}}`，渲染层无需区分。

---

## Entry 扩展字段

```go
type Entry struct {
    Key         string         `json:"key"`            // 必填：翻译键
    Value       string         `json:"value"`          // 必填：翻译文本，支持 {{.Var}} 模板
    Description string         `json:"desc,omitempty"` // 可选：人类可读描述
    Deprecated  bool           `json:"deprecated,omitempty"` // 可选：是否已废弃
    Tags        []string       `json:"tags,omitempty"` // 可选：分类标签
    Extra       map[string]any `json:"-"`              // 兜底：JSON 中所有未映射字段
}
```

`Key`、`Value`、`Description`、`Deprecated`、`Tags` 是 SDK 预定义的知名字段。JSON 中的任意自定义字段自动收入 `Extra` map，SDK 透传：

```json
{"key": "label", "value": "名称", "max_length": 50, "auditor": "张三"}
```

```go
entry, _ := bundle.Entry("zh", "label")
maxLen := entry.Extra["max_length"].(float64)  // 50
```

如需类型安全的扩展字段访问，使用 `DecodeEntries`：

```go
type MyEntry struct {
    Key       string `json:"key"`
    Value     string `json:"value"`
    MaxLength int    `json:"max_length"`
}
var entries []MyEntry
bundle.DecodeEntries("zh", &entries)
```

---

## Key 全量校验（StrictMode）

`StrictMode: true` 时，以**所有语言文件的 key 并集**为基准集，检查每种语言的 key 集合：

```
基准集 = zh.keys ∪ en.keys ∪ ja.keys

校验：
  zh 缺少 key: [X, Y]   → error
  en 缺少 key: [Z]      → error
  ja 完整              → ok
```

不指定"基准语言"——任何语言多出来的 key，其他语言也必须有。这避免了"zh 是权威，en 可以少"的隐性假设。

校验失败时返回结构化错误：

```
i18n: key validation failed (strict mode)
  [en] missing 2 keys: [error.not_found, btn.reset]
    files: en.json
  [ja] missing 1 key: [farewell]
    files: ja.json
  hint: add missing entries or set StrictMode=false
```

`StrictMode: false`（默认）时，缺少 key 不报错，适合开发阶段逐步补充翻译。

---

## 核心 API 参考

### 加载

```go
bundle, err := i18n.Load(cfg)     // 加载 + key 校验，返回 error
bundle := i18n.MustLoad(cfg)      // 同上，失败 panic
bundle := i18n.New("zh")          // 空 Bundle，后续 Add/AddFile 填充
```

### 翻译

```go
bundle.T("zh", "greeting")                              // "你好"
bundle.Tf("zh", "farewell", map[string]any{             // "再见 小明"
    "Name": "小明",
})
```

模板语法使用 Go `text/template`：`{{.Field}}`。模板编译结果按 `lang:key` 缓存。

### 请求级翻译器

```go
loc := bundle.L("zh")                                    // 绑定语言
loc.T("greeting")                                        // "你好"
loc.Tf("greet_name", map[string]any{"Name": "小明"})     // 带模板
loc.Lang()                                               // "zh"
```

### 查询

```go
bundle.Has("zh", "greeting")     // bool
bundle.Languages()               // []string，已排序
```

### 条目访问

```go
bundle.Entries("zh")                         // []Entry，按 key 排序
bundle.Entry("zh", "greeting")               // (Entry, bool)
bundle.DecodeEntries("zh", &customStruct)    // 自定义反序列化
```

### 动态追加

```go
bundle.Add("zh", []Entry{{Key: "new_key", Value: "新值"}})
bundle.AddFile("zh", "/path/to/zh_extra.json")  // 格式自动检测
```

---

## Config 选项

| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| `Dir` | `string` | 单一语言文件目录 | - |
| `Dirs` | `[]string` | 多个目录，按序加载，后覆盖前 | - |
| `FS` | `embed.FS` | embed 文件系统 | - |
| `SubDir` | `string` | FS 内子目录路径 | - |
| `Languages` | `[]string` | 仅加载指定语言前缀 | 全部 |
| `DefaultLang` | `string` | 默认语言（回退策略） | `"en"` |
| `StrictMode` | `bool` | key 缺失时 Load 是否报错 | `false` |

`FS`、`Dir`、`Dirs` 可组合使用（先加载 FS 中的默认翻译，再加载本地目录覆盖）。

---

## 接入示例

### 存量项目（零改造）

```go
// 现有 go-i18n 格式的 locales 目录，直接加载
var I18N = i18n.MustLoad(i18n.Config{
    Dir:         "./locales",    // zh.json 为 go-i18n 扁平格式
    DefaultLang: "zh",
    StrictMode:  false,          // 先不强制校验，逐步补全翻译后开启
})
msg := I18N.T("zh", "greeting")  // 行为与之前完全一致
```

### 新项目

```go
//go:embed locales/*
var localesFS embed.FS

var I18N = i18n.MustLoad(i18n.Config{
    FS:          localesFS,
    SubDir:      "locales",
    DefaultLang: "zh",
    StrictMode:  true,
})
```

### 渐进迁移

```go
// 目录同时存在新旧格式，逐步将旧文件改写为新格式
var I18N = i18n.MustLoad(i18n.Config{
    Dir:         "./locales",
    DefaultLang: "zh",
    StrictMode:  true,
})
// locales/zh.json        → 格式 2（go-i18n 扁平），存量
// locales/zh_errors.json → 格式 1（新数组），新增
// locales/en.json        → 格式 1（新数组），已迁移
```

### 与 web/lang 集成

```go
func MyHandler(c *gin.Context) {
    loc := lang.GetLocalizer(c, bundle)
    c.JSON(200, gin.H{"msg": loc.T("greeting")})
}
```

---

## 设计决策速查表

| 决策 | 选择 | 一句话理由 |
|------|------|-----------|
| 依赖 | 仅标准库 | encoding/json + text/template，无第三方库 |
| 语言识别 | 文件名前缀（首个下划线前段） | zh_errors.json → lang="zh"，与现有 zh.json 自然共存 |
| go-i18n 兼容 | 格式兼容，不依赖库 | 自动检测 go-i18n 扁平/数组 JSON，零外部依赖解析 |
| 新 JSON 格式 | 对象数组 [{key, value, ...}] | 支持扩展字段、保序、翻译人员友好 |
| Entry 扩展 | 固定字段 + Extra map | 90% 场景零额外代码，10% 场景走 DecodeEntries 逃生舱 |
| Key 校验 | 并集基准 + StrictMode 开关 | 杜绝漏翻译，开发期可关 |
| 全局变量 | MustLoad 返回实例 | 可测试、无包级状态、贴合项目风格 |
| 接口层 | 保留但不作为主 API | Go 隐式接口自然支持 Mock，无需显式导出 |
| 模板引擎 | text/template | 零依赖，语法完全兼容 go-i18n |
| 隐藏文件 | 自动过滤 `.` 开头文件 | 避免 macOS `._*.json`、`.DS_Store` 污染加载 |

---

## 源码文件结构

```
i18n/
├── entry.go         # Entry 结构体 + 自定义 JSON 反序列化（Extra 兜底）
├── normalize.go     # NormalizeLang + extractLang（语言前缀规则）
├── config.go        # Config 结构体
├── errors.go        # ValidationError + LoadError 结构化错误
├── loader.go        # 加载引擎：目录扫描 + 3 种格式自动检测 + key 校验
├── bundle.go        # Bundle 核心：T/Tf/Has/Languages/L/Entries/DecodeEntries/Add
├── localizer.go     # RequestLocalizer：请求级翻译器
├── interface.go     # Translator + Localizer 接口（web/lang 兼容）
├── i18n_test.go     # 31 个测试用例
└── README.md        # 本文档
```
