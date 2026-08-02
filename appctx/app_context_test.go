package appctx

import (
	"context"
	"errors"
	"testing"

	"github.com/izliang/bore/logit"
	"github.com/izliang/bore/logit/lctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLogitLogger 是 logit.Logger 的 mock 实现。
type MockLogitLogger struct {
	mock.Mock
}

func (m *MockLogitLogger) Write(p []byte) (n int, err error) {
	args := m.Called(p)
	return args.Int(0), args.Error(1)
}

func (m *MockLogitLogger) Info(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	m.Called(ctx, msg, fields)
}

func (m *MockLogitLogger) Debug(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	m.Called(ctx, msg, fields)
}

func (m *MockLogitLogger) Warn(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	m.Called(ctx, msg, fields)
}

func (m *MockLogitLogger) Error(ctx context.Context, err error, fields ...logit.FieldFunc) {
	m.Called(ctx, err, fields)
}

func (m *MockLogitLogger) Trace(ctx context.Context, msg string, fields ...logit.FieldFunc) {
	m.Called(ctx, msg, fields)
}

// ============================================================
// 构造和基本功能测试
// ============================================================

func TestNew_Default(t *testing.T) {
	tests := []struct {
		name  string
		opts  []Option
		check func(*testing.T, AppContext)
	}{
		{
			name: "no options - should have background context and default logger",
			opts: nil,
			check: func(t *testing.T, ac AppContext) {
				assert.NotNil(t, ac)
				assert.NotNil(t, ac.Logger())
				assert.NotNil(t, ac.Context())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac := New(tt.opts...)
			tt.check(t, ac)
		})
	}
}

func TestNew_WithOptions(t *testing.T) {
	customCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockLogger := &MockLogitLogger{}

	tests := []struct {
		name        string
		opts        []Option
		checkCtx    func(*testing.T, context.Context)
		checkLogger func(*testing.T, logit.Logger)
	}{
		{
			name: "WithContext sets custom context",
			opts: []Option{
				WithContext(customCtx),
			},
			checkCtx: func(t *testing.T, ctx context.Context) {
				ac := ctx.(AppContext)
				assert.Equal(t, customCtx, ac.Context())
			},
		},
		{
			name: "WithLogger sets custom logger",
			opts: []Option{
				WithLogger(mockLogger),
			},
			checkLogger: func(t *testing.T, l logit.Logger) {
				assert.Equal(t, mockLogger, l)
			},
		},
		{
			name: "WithContext and WithLogger both set",
			opts: []Option{
				WithContext(customCtx),
				WithLogger(mockLogger),
			},
			checkCtx: func(t *testing.T, ctx context.Context) {
				ac := ctx.(AppContext)
				assert.Equal(t, customCtx, ac.Context())
			},
			checkLogger: func(t *testing.T, l logit.Logger) {
				assert.Equal(t, mockLogger, l)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac := New(tt.opts...)

			if tt.checkCtx != nil {
				tt.checkCtx(t, ac)
			}

			if tt.checkLogger != nil {
				tt.checkLogger(t, ac.LogitLogger())
			}

			assert.NotNil(t, ac)
			assert.NotNil(t, ac.Logger())
			assert.NotNil(t, ac.Context())
		})
	}
}

func TestImpl_Methods(t *testing.T) {
	customCtx := context.Background()
	mockLogger := &MockLogitLogger{}

	ac := New(WithContext(customCtx), WithLogger(mockLogger))

	assert.Equal(t, customCtx, ac.Context())
	assert.Equal(t, mockLogger, ac.LogitLogger())
}

func TestAppContext_LoggingMethods(t *testing.T) {
	testCtx := context.Background()
	mockLogger := &MockLogitLogger{}
	ac := New(WithContext(testCtx), WithLogger(mockLogger))

	testErr := errors.New("test error")
	testField := logit.StringField("key", "value")

	tests := []struct {
		name   string
		method string
		msg    string
		err    error
		fields []logit.FieldFunc
		setup  func()
		verify func(*testing.T, *MockLogitLogger)
	}{
		{
			name:   "AppContext.Info delegates to logit.Info with current context",
			method: "Info",
			msg:    "test info",
			fields: []logit.FieldFunc{testField},
			setup: func() {
				mockLogger.On("Info", testCtx, "test info", mock.AnythingOfType("[]logit.FieldFunc")).Return()
			},
			verify: func(t *testing.T, m *MockLogitLogger) {
				m.AssertExpectations(t)
			},
		},
		{
			name:   "AppContext.Debug delegates to logit.Debug with current context",
			method: "Debug",
			msg:    "test debug",
			fields: []logit.FieldFunc{testField},
			setup: func() {
				mockLogger.On("Debug", testCtx, "test debug", mock.AnythingOfType("[]logit.FieldFunc")).Return()
			},
			verify: func(t *testing.T, m *MockLogitLogger) {
				m.AssertExpectations(t)
			},
		},
		{
			name:   "AppContext.Warn delegates to logit.Warn with current context",
			method: "Warn",
			msg:    "test warn",
			fields: []logit.FieldFunc{testField},
			setup: func() {
				mockLogger.On("Warn", testCtx, "test warn", mock.AnythingOfType("[]logit.FieldFunc")).Return()
			},
			verify: func(t *testing.T, m *MockLogitLogger) {
				m.AssertExpectations(t)
			},
		},
		{
			name:   "AppContext.Error delegates to logit.Error with current context",
			method: "Error",
			err:    testErr,
			fields: []logit.FieldFunc{testField},
			setup: func() {
				mockLogger.On("Error", testCtx, testErr, mock.AnythingOfType("[]logit.FieldFunc")).Return()
			},
			verify: func(t *testing.T, m *MockLogitLogger) {
				m.AssertExpectations(t)
			},
		},
		{
			name:   "Logger.Info delegates to logit.Info with current context (backward compat)",
			method: "LoggerInfo",
			msg:    "test info",
			fields: []logit.FieldFunc{testField},
			setup: func() {
				mockLogger.On("Info", testCtx, "test info", mock.AnythingOfType("[]logit.FieldFunc")).Return()
			},
			verify: func(t *testing.T, m *MockLogitLogger) {
				m.AssertExpectations(t)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLogger.ExpectedCalls = nil
			mockLogger.Calls = nil

			tt.setup()

			switch tt.method {
			case "Info":
				if tt.err != nil {
					ac.Error(tt.err, tt.fields...)
				} else {
					ac.Info(tt.msg, tt.fields...)
				}
			case "Debug":
				ac.Debug(tt.msg, tt.fields...)
			case "Warn":
				ac.Warn(tt.msg, tt.fields...)
			case "Error":
				ac.Error(tt.err, tt.fields...)
			case "LoggerInfo":
				ac.Logger().Info(tt.msg, tt.fields...)
			}

			tt.verify(t, mockLogger)
		})
	}
}

func TestAppContext_With_ReturnsNewInstanceWithNewContext(t *testing.T) {
	// 核心契约：With 返回新的 AppContext 携带新 context，原始值不变
	originalCtx := context.Background()
	mockLogger := &MockLogitLogger{}

	original := New(WithContext(originalCtx), WithLogger(mockLogger))

	newCtx, cancel := context.WithCancel(originalCtx)
	defer cancel()

	updated := original.With(newCtx)

	// 原始值不变
	assert.Equal(t, originalCtx, original.Context())
	assert.Equal(t, mockLogger, original.LogitLogger())

	// 新值有新的 context 和相同的 logger
	assert.Equal(t, newCtx, updated.Context())
	assert.Equal(t, mockLogger, updated.LogitLogger())
}

func TestAppContext_With_PreservesLogger(t *testing.T) {
	originalCtx := context.Background()
	mockLogger := &MockLogitLogger{}
	original := New(WithContext(originalCtx), WithLogger(mockLogger))

	newCtx := context.WithValue(originalCtx, "key", "value")
	updated := original.With(newCtx)

	assert.Equal(t, mockLogger, updated.LogitLogger())
}

func TestAppContext_LctxPropagation_WorksCorrectly(t *testing.T) {
	// 验证：lctx.PushPair 添加字段后，日志应携带这些字段
	mockLogger := &MockLogitLogger{}
	ac := New(WithLogger(mockLogger))

	testFormatter := &testLogFormatter{
		values: []lctx.Pair{{Key: "request_id", Value: "abc-123"}},
	}

	// PushPair 返回新 context
	acWithField := ac.With(lctx.PushPair(ac.Context(), testFormatter))

	// Info 应使用携带字段的 context
	mockLogger.On("Info", acWithField.Context(), "test message", mock.Anything).Return()

	acWithField.Info("test message")

	mockLogger.AssertExpectations(t)

	// 原始 appctx 不变，没有新字段
	mockLogger.On("Info", ac.Context(), "original message", mock.Anything).Return()
	ac.Info("original message")
	mockLogger.AssertExpectations(t)
}

func TestAppContext_LctxPropagation_ThroughLogger(t *testing.T) {
	// 通过 Logger() 门面调用时，也应使用更新后的 context
	mockLogger := &MockLogitLogger{}
	ac := New(WithLogger(mockLogger))

	testFormatter := &testLogFormatter{
		values: []lctx.Pair{{Key: "request_id", Value: "abc-123"}},
	}

	acWithField := ac.With(lctx.PushPair(ac.Context(), testFormatter))

	mockLogger.On("Info", acWithField.Context(), "test via Logger", mock.Anything).Return()

	acWithField.Logger().Info("test via Logger")

	mockLogger.AssertExpectations(t)
}

// ============================================================
// 基础组件（GORM / Redis）测试
// ============================================================

func TestAppContext_GORM_ReturnsNilByDefault(t *testing.T) {
	ac := New()
	assert.Nil(t, ac.GORM(), " WithGORM  nil")
}

func TestAppContext_Redis_ReturnsNilByDefault(t *testing.T) {
	ac := New()
	assert.Nil(t, ac.Redis(), " WithRedis  nil")
}

func TestAppContext_With_PreservesGORMAndRedis(t *testing.T) {
	ac := New()

	// GORM / Redis 默认为 nil
	assert.Nil(t, ac.GORM())
	assert.Nil(t, ac.Redis())

	// With 后 GORM/Redis 仍为 nil（不会变成其他值）
	newCtx := context.WithValue(context.Background(), "key", "value")
	ac2 := ac.With(newCtx)

	assert.Nil(t, ac2.GORM(), "With GORM should nil")
	assert.Nil(t, ac2.Redis(), "With Redis should nil")
	assert.Equal(t, newCtx, ac2.Context())
}

// ============================================================
// nil 安全测试 —— 构造阶段 fail-fast
// ============================================================

func TestWithContext_NilPanics(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "WithContext(nil) should panic")
		assert.Contains(t, r.(string), "WithContext")
	}()
	New(WithContext(nil))
}

func TestWith_NilPanics(t *testing.T) {
	ac := New()
	defer func() {
		r := recover()
		assert.NotNil(t, r, "With(nil) should panic")
		assert.Contains(t, r.(string), "With")
	}()
	ac.With(nil)
}

func TestWithLogger_NilPanics(t *testing.T) {
	defer func() {
		r := recover()
		assert.NotNil(t, r, "WithLogger(nil) should panic")
		assert.Contains(t, r.(string), "WithLogger")
	}()
	New(WithLogger(nil))
}

// ============================================================
// nil 安全测试 —— 运行时 noop 降级
// ============================================================

// nilLoggerAppContext 构造一个 logger 为 nil 的 AppContext，
// 用于验证日志方法在 logger 为 nil 时不会 panic。
// 正常构造路径（New + WithLogger）已禁止 nil logger，
// 这里直接构造 impl 模拟极端场景（如序列化/反序列化后字段丢失）。
func nilLoggerAppContext() AppContext {
	return &impl{
		ctx:    context.Background(),
		logger: nil,
	}
}

func TestLogMethods_NilLogger_Noop(t *testing.T) {
	ac := nilLoggerAppContext()

	// 所有日志方法不应 panic
	assert.NotPanics(t, func() { ac.Info("test") })
	assert.NotPanics(t, func() { ac.Debug("test") })
	assert.NotPanics(t, func() { ac.Warn("test") })
	assert.NotPanics(t, func() { ac.Error(errors.New("err")) })

	// Logger 门面也应安全
	assert.NotPanics(t, func() { ac.Logger().Info("test") })
}

func TestLogMethods_NilLogger_DoesNotCallUnderlyingLogger(t *testing.T) {
	// 底层 logger 为 nil 时不应被调用（否则 nil receiver 调用会 panic）
	ac := nilLoggerAppContext()
	assert.NotPanics(t, func() {
		ac.Info("test", logit.StringField("key", "value"))
		ac.Debug("test", logit.StringField("key", "value"))
		ac.Warn("test", logit.StringField("key", "value"))
		ac.Error(errors.New("err"), logit.StringField("key", "value"))
	})
}

// ============================================================
// 辅助类型
// ============================================================

type testLogFormatter struct {
	values []lctx.Pair
}

func (t *testLogFormatter) Values() []lctx.Pair {
	return t.values
}
