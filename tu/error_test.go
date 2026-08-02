package tu

import (
	"errors"
	"fmt"
	"testing"
)

// --- errIsNil 测试 ---

func TestErrIsNil(t *testing.T) {
	// 纯 nil
	if !errIsNil(nil) {
		t.Error("nil should be nil")
	}

	// 普通错误
	if errIsNil(errors.New("boom")) {
		t.Error("non-nil error should not be nil")
	}

	// typed nil 指针 —— 经典陷阱
	var typedNil *myError = nil
	var e error = typedNil
	if e == nil {
		t.Error("typed nil pointer: e == nil unexpectedly true (precondition failed)")
	}
	if !errIsNil(e) {
		t.Error("typed nil pointer should be treated as nil by errIsNil")
	}

	// 非 nil 指针
	e2 := &myError{msg: "boom"}
	if errIsNil(e2) {
		t.Error("non-nil pointer should not be nil")
	}
}

type myError struct{ msg string }

func (m *myError) Error() string { return m.msg }

// --- CompareErr 测试 ---

func TestCompareErr_BothNil(t *testing.T) {
	r := CompareErr(nil, nil)
	if !r.Match {
		t.Errorf("both nil should match, got: %s", r.Diff)
	}
}

func TestCompareErr_TypedNil(t *testing.T) {
	var typedNil *myError = nil
	r := CompareErr(typedNil, nil)
	if !r.Match {
		t.Errorf("typed nil vs nil should match, got: %s", r.Diff)
	}
}

func TestCompareErr_OneNil(t *testing.T) {
	r := CompareErr(errors.New("boom"), nil)
	if r.Match {
		t.Error("non-nil vs nil should not match")
	}

	r2 := CompareErr(nil, errors.New("boom"))
	if r2.Match {
		t.Error("nil vs non-nil should not match")
	}
}

func TestCompareErr_StringMatch(t *testing.T) {
	r := CompareErr(errors.New("boom"), errors.New("boom"))
	if !r.Match {
		t.Errorf("same message should match, got: %s", r.Diff)
	}
}

func TestCompareErr_StringMismatch(t *testing.T) {
	r := CompareErr(errors.New("foo"), errors.New("bar"))
	if r.Match {
		t.Error("different messages should not match")
	}
	// 验证 diff 内容
	if r.Diff == "" {
		t.Error("diff should not be empty on mismatch")
	}
}

func TestCompareErr_ErrorsIsMatch(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := fmt.Errorf("wrap: %w", sentinel)

	r := CompareErr(wrapped, sentinel)
	if !r.Match {
		t.Errorf("wrapped error should match sentinel via errors.Is, got: %s", r.Diff)
	}
	// 反向
	r2 := CompareErr(sentinel, wrapped)
	if !r2.Match {
		t.Errorf("sentinel should match wrapped via errors.Is, got: %s", r2.Diff)
	}
}

func TestCompareErr_DoubleWrap(t *testing.T) {
	base := errors.New("base")
	inner := fmt.Errorf("inner: %w", base)
	outer := fmt.Errorf("outer: %w", inner)

	r := CompareErr(outer, base)
	if !r.Match {
		t.Errorf("double-wrapped should match base via errors.Is, got: %s", r.Diff)
	}
}

func TestCompareErr_ErrorsIsNoMatch(t *testing.T) {
	r := CompareErr(errors.New("foo"), errors.New("bar"))
	if r.Match {
		t.Error("unrelated errors should not match")
	}
}

// --- generateDiff 测试 ---

func TestGenerateDiff_Basic(t *testing.T) {
	diff := generateDiff("hello", "hallo")
	// 应该定位到位置 1（e vs a）
	if diff == "" {
		t.Error("diff should not be empty")
	}
	t.Logf("basic diff:\n%s", diff)
}

func TestGenerateDiff_UTF8(t *testing.T) {
	// 中文多字节字符 —— 不应 panic，不应显示乱码
	diff := generateDiff("你好世界", "你好地球")
	if diff == "" {
		t.Error("diff should not be empty")
	}
	// 确认没有乱码（包含合法中文字符）
	t.Logf("UTF-8 diff:\n%s", diff)
}

func TestGenerateDiff_PrefixMismatch(t *testing.T) {
	// actual="hello", expected="hello world" —— 之前漏报差异
	diff := generateDiff("hello", "hello world")
	t.Logf("prefix diff:\n%s", diff)
	// 应包含 "extra" 或 "Length mismatch" 或位置信息
}

func TestGenerateDiff_EmptyString(t *testing.T) {
	diff := generateDiff("", "abc")
	if diff == "" {
		t.Error("diff should not be empty for empty vs non-empty")
	}
	t.Logf("empty diff:\n%s", diff)
}

func TestGenerateDiff_SameLengthDifferentContent(t *testing.T) {
	diff := generateDiff("abcde", "abXde")
	// 位置 2 不同
	t.Logf("same-length diff:\n%s", diff)
}

// --- ValidateErr 测试 ---

func TestValidateErr_WantErrGotErr(t *testing.T) {
	ok, msg := ValidateErr(errors.New("boom"), true)
	if !ok {
		t.Errorf("wantErr=true with error should be ok, got: %s", msg)
	}
}

func TestValidateErr_WantErrGotNil(t *testing.T) {
	ok, msg := ValidateErr(nil, true)
	if ok {
		t.Errorf("wantErr=true with nil should not be ok, got: %s", msg)
	}
}

func TestValidateErr_WantNoErrGotNil(t *testing.T) {
	ok, msg := ValidateErr(nil, false)
	if !ok {
		t.Errorf("wantErr=false with nil should be ok, got: %s", msg)
	}
}

func TestValidateErr_WantNoErrGotErr(t *testing.T) {
	ok, msg := ValidateErr(errors.New("boom"), false)
	if ok {
		t.Errorf("wantErr=false with error should not be ok, got: %s", msg)
	}
}

func TestValidateErr_TypedNil(t *testing.T) {
	var typedNil *myError = nil
	ok, _ := ValidateErr(typedNil, false)
	if !ok {
		t.Error("typed nil with wantErr=false should be ok")
	}
}

// --- ValidateErrWithDetail 测试 ---

func TestValidateErrWithDetail_AllBranchesReturnResult(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"wantErr+hasErr", errors.New("boom"), true},
		{"wantErr+noErr", nil, true},
		{"noWantErr+noErr", nil, false},
		{"noWantErr+hasErr", errors.New("boom"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, result := ValidateErrWithDetail(tt.err, tt.wantErr)
			if result == nil {
				t.Errorf("%s: ErrDiffResult should not be nil", tt.name)
			}
		})
	}
}

// --- IsErrorType 测试 ---

func TestIsErrorType(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := fmt.Errorf("wrap: %w", sentinel)

	if !IsErrorType(wrapped, sentinel) {
		t.Error("wrapped should match sentinel")
	}
	if IsErrorType(errors.New("other"), sentinel) {
		t.Error("unrelated should not match")
	}
}

// --- AssertError 测试 ---

func TestAssertError_Match(t *testing.T) {
	if !AssertError(t, nil, nil) {
		t.Error("nil vs nil should assert ok")
	}
}

func TestAssertError_ErrorsIsMatch(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := fmt.Errorf("wrap: %w", sentinel)
	if !AssertError(t, wrapped, sentinel) {
		t.Error("wrapped should assert ok via errors.Is")
	}
}

// --- TestingT 接口兼容性验证 ---

func TestTestingTSatisfied(t *testing.T) {
	// 编译期验证：testing.T 满足 TestingT 接口
	var _ TestingT = t
}

// --- AssertWantError 测试 ---

func TestAssertWantError_WantErrGotErr(t *testing.T) {
	if !AssertWantError(t, errors.New("boom"), true) {
		t.Error("wantErr=true with error should assert ok")
	}
}

func TestAssertWantError_WantErrGotNil(t *testing.T) {
	// 用 fakeT 捕获 Errorf，避免测试框架捕获失败
	ft := &fakeT{}
	if AssertWantError(ft, nil, true) {
		t.Error("wantErr=true with nil should not assert ok")
	}
	if !ft.called {
		t.Error("should have called Errorf")
	}
}

func TestAssertWantError_WantNoErrGotNil(t *testing.T) {
	if !AssertWantError(t, nil, false) {
		t.Error("wantErr=false with nil should assert ok")
	}
}

func TestAssertWantError_WantNoErrGotErr(t *testing.T) {
	ft := &fakeT{}
	if AssertWantError(ft, errors.New("boom"), false) {
		t.Error("wantErr=false with error should not assert ok")
	}
	if !ft.called {
		t.Error("should have called Errorf")
	}
}

func TestAssertWantError_TypedNil(t *testing.T) {
	var typedNil *myError = nil
	if !AssertWantError(t, typedNil, false) {
		t.Error("typed nil with wantErr=false should assert ok")
	}
}

// fakeT 实现 TestingT 接口，用于捕获 Errorf 调用
type fakeT struct {
	called bool
}

func (f *fakeT) Helper()                                          {}
func (f *fakeT) Errorf(format string, args ...interface{}) { f.called = true }
