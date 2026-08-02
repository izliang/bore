// Package tu 提供测试辅助工具，包括错误比较、验证和断言。
//
// 本包的核心功能：
//   - CompareErr: 比较两个 error，支持字符串匹配和 errors.Is 语义匹配
//   - ValidateErr / ValidateErrWithDetail: 验证是否符合"有/无错误"预期
//   - AssertError: 一行断言，适用于 Table-Driven Test
//   - IsErrorType: errors.Is 的便捷封装
//   - TestingT: testing.TB 的轻量接口
package tu

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// ErrDiffResult 包含错误比较的结果和差异信息。
type ErrDiffResult struct {
	Match       bool   // 是否匹配
	Diff        string // 差异描述（人类可读，可直接用于日志或测试输出）
	ActualErr   error  // 实际错误
	ExpectedErr error  // 期望错误
}

// errIsNil 安全判断 error 接口值是否为 nil。
//
// 防御 Go 经典陷阱：typed nil 指针赋值给 error interface 后，
// interface 本身不为 nil（持有 *MyError 类型信息），但底层值为 nil。
//
//	var e *MyError = nil
//	e == nil              // false（interface 持有类型信息）
//	errIsNil(e)           // true（通过反射检查底层值）
//
// 仅当 err 为 nil interface 或底层可 nil 类型（指针/接口/channel/func/map/slice）的 nil 值时返回 true。
func errIsNil(err error) bool {
	if err == nil {
		return true
	}
	v := reflect.ValueOf(err)
	switch v.Kind() {
	case reflect.Ptr, reflect.Interface, reflect.Chan, reflect.Func, reflect.Map, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// CompareErr 比较实际错误和期望错误，返回详细的差异信息。
//
// 比较策略（按优先级，命中即停止）：
//  1. 两者经 errIsNil 判定均为 nil  → 匹配
//  2. 一个为 nil 另一个非 nil        → 不匹配
//  3. Error() 字符串完全相同          → 匹配
//  4. errors.Is(actual, expected)    → 语义匹配（actual 包装了 expected 或与 expected 相同）
//  5. errors.Is(expected, actual)    → 语义匹配（expected 包装了 actual）
//  6. 以上均不满足                    → 不匹配，生成详细字符级差异报告
//
// 策略 4-5 确保 wrapped error 场景也能正确匹配：
//
//	var ErrMy = errors.New("my error")
//	actual := fmt.Errorf("wrap: %w", ErrMy)
//	CompareErr(actual, ErrMy).Match  // true，Diff 中注明 "via errors.Is"
func CompareErr(actual, expected error) *ErrDiffResult {
	result := &ErrDiffResult{
		ActualErr:   actual,
		ExpectedErr: expected,
	}

	actualNil := errIsNil(actual)
	expectedNil := errIsNil(expected)

	if actualNil && expectedNil {
		result.Match = true
		result.Diff = "both errors are nil"
		return result
	}

	if actualNil {
		result.Match = false
		result.Diff = fmt.Sprintf("expected error '%v' but got nil", expected)
		return result
	}

	if expectedNil {
		result.Match = false
		result.Diff = fmt.Sprintf("got error '%v' but expected nil", actual)
		return result
	}

	actualMsg := actual.Error()
	expectedMsg := expected.Error()

	if actualMsg == expectedMsg {
		result.Match = true
		result.Diff = fmt.Sprintf("both errors match: %q", actualMsg)
		return result
	}

	// 字符串不匹配时，尝试 errors.Is 语义匹配
	if errors.Is(actual, expected) {
		result.Match = true
		result.Diff = fmt.Sprintf(
			"errors match via errors.Is (actual wraps expected)\n  Expected: %q\n  Actual:   %q",
			expectedMsg, actualMsg,
		)
		return result
	}
	if errors.Is(expected, actual) {
		result.Match = true
		result.Diff = fmt.Sprintf(
			"errors match via errors.Is (expected wraps actual)\n  Expected: %q\n  Actual:   %q",
			expectedMsg, actualMsg,
		)
		return result
	}

	result.Match = false
	result.Diff = generateDiff(actualMsg, expectedMsg)
	return result
}

// generateDiff 生成两个字符串的差异对比（字符级，兼容 UTF-8）。
func generateDiff(actual, expected string) string {
	var b strings.Builder
	b.WriteString("error mismatch:\n")
	b.WriteString(fmt.Sprintf("  Expected: %q\n", expected))
	b.WriteString(fmt.Sprintf("  Actual:   %q\n", actual))

	// 使用 rune 切片保证多字节 UTF-8 字符不被截断
	actualR := []rune(actual)
	expectedR := []rune(expected)

	if len(actualR) != len(expectedR) {
		b.WriteString(fmt.Sprintf("  Length mismatch: expected %d chars, got %d chars\n",
			len(expectedR), len(actualR)))
	}

	minLen := len(actualR)
	if len(expectedR) < minLen {
		minLen = len(expectedR)
	}

	firstDiffPos := -1
	for i := 0; i < minLen; i++ {
		if actualR[i] != expectedR[i] {
			firstDiffPos = i
			break
		}
	}

	// 公共部分完全一致但长度不同 → 分歧点在较短字符串末尾之后
	if firstDiffPos == -1 && len(actualR) != len(expectedR) {
		firstDiffPos = minLen
	}

	if firstDiffPos >= 0 {
		ctxRadius := 10
		start := firstDiffPos - ctxRadius
		if start < 0 {
			start = 0
		}
		endExp := firstDiffPos + ctxRadius
		if endExp > len(expectedR) {
			endExp = len(expectedR)
		}
		endAct := firstDiffPos + ctxRadius
		if endAct > len(actualR) {
			endAct = len(actualR)
		}

		b.WriteString(fmt.Sprintf("  First difference at rune position %d:\n", firstDiffPos))
		b.WriteString(fmt.Sprintf("    Expected: ...%s...\n", string(expectedR[start:endExp])))
		b.WriteString(fmt.Sprintf("    Actual:   ...%s...\n", string(actualR[start:endAct])))

		// 显示具体差异字符
		inExp := firstDiffPos < len(expectedR)
		inAct := firstDiffPos < len(actualR)
		if inExp && inAct {
			b.WriteString(fmt.Sprintf("    Diff char: expected %q (U+%04X), got %q (U+%04X)\n",
				string(expectedR[firstDiffPos]), expectedR[firstDiffPos],
				string(actualR[firstDiffPos]), actualR[firstDiffPos]))
		} else if inExp {
			b.WriteString(fmt.Sprintf("    Expected has extra chars: %q\n", string(expectedR[firstDiffPos:])))
		} else {
			b.WriteString(fmt.Sprintf("    Actual has extra chars: %q\n", string(actualR[firstDiffPos:])))
		}
	}

	return b.String()
}

// ValidateErr 验证实际错误是否符合预期。
//
// wantErr 为 true 时期望有错误，为 false 时期望无错误。
// 返回 (是否符合预期, 描述信息)。
func ValidateErr(actual error, wantErr bool) (bool, string) {
	hasErr := !errIsNil(actual)

	if wantErr && hasErr {
		return true, fmt.Sprintf("got expected error: %v", actual)
	}
	if wantErr && !hasErr {
		return false, "expected error but got nil"
	}
	if !wantErr && !hasErr {
		return true, "no error as expected"
	}
	return false, fmt.Sprintf("expected no error but got: %v", actual)
}

// ValidateErrWithDetail 验证错误是否符合预期，同时返回详细差异。
//
// 相比 ValidateErr，额外返回 *ErrDiffResult 提供结构化差异信息。
// 所有分支均返回非 nil 的 *ErrDiffResult，调用方无需判空。
func ValidateErrWithDetail(actual error, wantErr bool) (bool, string, *ErrDiffResult) {
	hasErr := !errIsNil(actual)

	if wantErr && hasErr {
		return true, fmt.Sprintf("got expected error: %v", actual), &ErrDiffResult{
			Match:       true,
			Diff:        fmt.Sprintf("got expected error: %v", actual),
			ActualErr:   actual,
			ExpectedErr: nil, // wantErr 模式下不指定具体期望错误
		}
	}

	if wantErr && !hasErr {
		return false, "expected error but got nil", &ErrDiffResult{
			Match:       false,
			Diff:        "expected error but got nil",
			ActualErr:   nil,
			ExpectedErr: errors.New("non-nil error expected"), // wantErr=true 时占位
		}
	}

	if !wantErr && !hasErr {
		return true, "no error as expected", &ErrDiffResult{
			Match:       true,
			Diff:        "no error as expected",
			ActualErr:   nil,
			ExpectedErr: nil,
		}
	}

	// !wantErr && hasErr
	return false, fmt.Sprintf("expected no error but got: %v", actual), &ErrDiffResult{
		Match:       false,
		Diff:        fmt.Sprintf("expected no error but got: %v", actual),
		ActualErr:   actual,
		ExpectedErr: nil,
	}
}

// IsErrorType 检查 err 是否匹配 target，等价于 errors.Is(err, target)。
//
// 作为便捷封装供测试代码使用，使 API 保持一致。
// 对于需要 errors.As 的场景，请直接使用标准库。
func IsErrorType(err error, target error) bool {
	return errors.Is(err, target)
}

// AssertError 断言 actual 与 expected 匹配，适用于 Table-Driven Test。
//
// 使用 CompareErr 进行比较，不匹配时通过 t.Errorf 报告详细差异。
// 返回 true 表示匹配，false 表示不匹配。
//
// 典型用法：
//
//	tests := []struct {
//	    name    string
//	    input   any
//	    wantErr error
//	}{
//	    {name: "success", input: validInput, wantErr: nil},
//	    {name: "not found", input: badInput, wantErr: ErrNotFound},
//	}
//	for _, tt := range tests {
//	    t.Run(tt.name, func(t *testing.T) {
//	        err := DoSomething(tt.input)
//	        t.AssertError(t, err, tt.wantErr)
//	    })
//	}
func AssertError(t TestingT, actual, expected error) bool {
	t.Helper()
	result := CompareErr(actual, expected)
	if !result.Match {
		t.Errorf("Error mismatch:\n%s", result.Diff)
	}
	return result.Match
}


// AssertWantError 根据 wantErr 标志断言是否有错误，适用于 Table-Driven Test。
//
// wantErr 为 true 时期望有错误，为 false 时期望无错误。
// 内部调用 ValidateErr 进行判断，不匹配时通过 t.Errorf 报告。
// 返回 true 表示符合预期。
//
// 典型用法：
//
//	tests := []struct {
//	    name    string
//	    input   any
//	    wantErr bool
//	}{
//	    {name: "success", input: validInput, wantErr: false},
//	    {name: "error case", input: badInput, wantErr: true},
//	}
//	for _, tt := range tests {
//	    t.Run(tt.name, func(t *testing.T) {
//	        err := DoSomething(tt.input)
//	        t.AssertWantError(t, err, tt.wantErr)
//	    })
//	}
func AssertWantError(t TestingT, actual error, wantErr bool) bool {
	t.Helper()
	ok, msg := ValidateErr(actual, wantErr)
	if !ok {
		t.Errorf("Error expectation mismatch:\n  %s", msg)
	}
	return ok
}
// TestingT 是 testing.TB 的轻量接口，使本包不直接依赖 testing。
// testing.T、testing.B 和 testing.F 均满足此接口。
type TestingT interface {
	Helper()
	Errorf(format string, args ...interface{})
}
