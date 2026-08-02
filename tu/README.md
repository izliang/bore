# tu — Go 测试辅助包

对 `errors` 标准库的增强封装，提供错误比较、验证、断言等测试常用操作。适用于 **Table-Driven Test** 和普通单元测试场景。

## 快速开始

```go
import "github.com/izliang/bore/tu"
```

## API 概览

| 函数 | 用途 | 适用场景 |
|---|---|---|
| `CompareErr(actual, expected)` | 深度比较两个 error | 精确断言错误内容 |
| `ValidateErr(actual, wantErr)` | 验证有/无错误 | 只需判断是否有错 |
| `ValidateErrWithDetail(actual, wantErr)` | 同上 + 结构化差异 | 需要获取差异详情 |
| `AssertError(t, actual, expected)` | 断言错误匹配 | Table-Driven Test（指定错误） |
| `AssertWantError(t, actual, wantErr)` | 断言有/无错误 | Table-Driven Test（bool 标志） |
| `IsErrorType(err, target)` | 等价于 `errors.Is` | API 一致性 |

## 核心特性

### 1. 多层匹配策略 — `CompareErr`

`CompareErr` 按优先级依次尝试匹配，命中即停止：

1. 两者均为 nil → 匹配
2. 一个为 nil 另一个非 nil → 不匹配
3. `Error()` 字符串完全相同 → 匹配
4. `errors.Is(actual, expected)` → 匹配（**wrapped error 自动识别**）
5. `errors.Is(expected, actual)` → 匹配
6. 以上都不满足 → 不匹配，输出字符级差异报告

```go
// 基本字符串匹配
r := tu.CompareErr(errors.New("boom"), errors.New("boom"))
fmt.Println(r.Match) // true

// wrapped error 语义匹配
var ErrNotFound = errors.New("not found")
actual := fmt.Errorf("user query: %w", ErrNotFound)
r = tu.CompareErr(actual, ErrNotFound)
fmt.Println(r.Match) // true, Diff 注明 "via errors.Is"
```

### 2. 防御 nil interface 陷阱

Go 中 typed nil 指针赋值给 `error` interface 后 `== nil` 为 false。本包所有函数通过反射安全检查底层值，正确识别 typed nil：

```go
var typedNil *MyError = nil
typedNil == nil            // false（陷阱）
tu.CompareErr(typedNil, nil).Match // true（正确识别）
```

### 3. UTF-8 安全的差异报告

差异输出基于 `rune` 而非 byte，中文、emoji 等多字节字符不会产生乱码：

```
error mismatch:
  Expected: "你好地球"
  Actual:   "你好世界"
  First difference at rune position 2:
    Expected: ...你好地球...
    Actual:   ...你好世界...
    Diff char: expected "地" (U+5730), got "世" (U+4E16)
```

### 4. Table-Driven Test 友好

```go
tests := []struct {
    name    string
    input   string
    wantErr error
}{
    {name: "success", input: "valid", wantErr: nil},
    {name: "missing", input: "",     wantErr: ErrRequired},
    {name: "too long", input: "xxx", wantErr: ErrTooLong},
}

for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        err := Validate(tt.input)
        tu.AssertError(t, err, tt.wantErr)
    })
}
```

## 类型定义

### `ErrDiffResult`

```go
type ErrDiffResult struct {
    Match       bool   // 是否匹配
    Diff        string // 差异描述（可直接用于日志）
    ActualErr   error  // 实际错误
    ExpectedErr error  // 期望错误
}
```

### `TestingT`

`testing.TB` 的轻量接口，`testing.T`、`testing.B`、`testing.F` 均满足：

```go
type TestingT interface {
    Helper()
    Errorf(format string, args ...interface{})
}
```

## 函数详解

### `CompareErr`

```go
func CompareErr(actual, expected error) *ErrDiffResult
```

比较实际错误和期望错误。**始终返回非 nil 结果**，通过 `result.Match` 判断。

### `ValidateErr`

```go
func ValidateErr(actual error, wantErr bool) (ok bool, msg string)
```

- `wantErr=true`：期望有错误，实际有错误 → ok
- `wantErr=false`：期望无错误，实际无错误 → ok

### `ValidateErrWithDetail`

```go
func ValidateErrWithDetail(actual error, wantErr bool) (ok bool, msg string, result *ErrDiffResult)
```

`ValidateErr` 的增强版，额外返回结构化的 `ErrDiffResult`。**所有分支均返回非 nil result**。

### `AssertError`

```go
func AssertError(t TestingT, actual, expected error) bool
```

内部调用 `CompareErr`，不匹配时通过 `t.Errorf` 输出差异。

### `AssertWantError`

```go
func AssertWantError(t TestingT, actual error, wantErr bool) bool
```

适用于 `wantErr bool` 风格的 Table-Driven Test。内部调用 `ValidateErr`，不匹配时通过 `t.Errorf` 报告。

```go
tests := []struct {
    name    string
    input   string
    wantErr bool
}{
    {name: "success", input: "valid", wantErr: false},
    {name: "missing", input: "",     wantErr: true},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        err := Validate(tt.input)
        tu.AssertWantError(t, err, tt.wantErr)
    })
}
```

### `IsErrorType`

```go
func IsErrorType(err, target error) bool
```

等价于 `errors.Is(err, target)`，作为常量封装保留 API 一致性。

## 与 testify 的关系

本包**不替代** `github.com/stretchr/testify`，而是与其互补：

- `testify/assert` 提供通用断言（`Equal`, `NoError`, `ErrorIs` 等）
- `tu` 包专注 **error 场景**，提供更丰富的差异信息和 wrapped error 语义匹配

两者可在同一测试文件中混用。
