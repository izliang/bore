package appctx

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// MockAppContext 基本功能
// ============================================================

func TestNewMock_Defaults(t *testing.T) {
	mock := NewMock()

	// 默认值
	assert.NotNil(t, mock.Context())
	assert.Nil(t, mock.LogitLogger())
	assert.Nil(t, mock.GORM())
	assert.Nil(t, mock.Redis())
	assert.Nil(t, mock.SQLMock)

	// Logger() 返回自身
	assert.Equal(t, mock, mock.Logger())
}

func TestNewMock_WithMockContext(t *testing.T) {
	customCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock := NewMock(WithMockContext(customCtx))
	assert.Equal(t, customCtx, mock.Context())
}

func TestNewMock_WithMockGORM(t *testing.T) {
	gormDB, _, err := NewSQLMock()
	require.NoError(t, err)
	defer CloseSQLMock(gormDB)

	mock := NewMock(WithMockGORM(gormDB))
	assert.Equal(t, gormDB, mock.GORM())
}

func TestNewMock_With_PropagatesAllFields(t *testing.T) {
	gormDB, _, err := NewSQLMock()
	require.NoError(t, err)
	defer CloseSQLMock(gormDB)

	mock := NewMock(WithMockGORM(gormDB))
	require.NotNil(t, mock.GORM())

	newCtx := context.WithValue(context.Background(), "key", "value")
	mock2 := mock.With(newCtx)

	assert.Equal(t, newCtx, mock2.Context())
	assert.Equal(t, gormDB, mock2.GORM())
	assert.Equal(t, mock.LogitLogger(), mock2.LogitLogger())

	// 原始 mock 不变
	assert.NotEqual(t, newCtx, mock.Context())
}

// ============================================================
// MockAppContext —— nil 安全检查
// ============================================================

func TestMockAppContext_NilCtx_ContextReturnsBackground(t *testing.T) {
	mock := &MockAppContext{Ctx: nil}
	assert.NotNil(t, mock.Context())
	assert.Equal(t, context.Background(), mock.Context())
}

func TestMockAppContext_NilCtx_DeadlineReturnsZero(t *testing.T) {
	mock := &MockAppContext{Ctx: nil}
	d, ok := mock.Deadline()
	assert.False(t, ok)
	assert.True(t, d.IsZero())
}

func TestMockAppContext_NilCtx_DoneReturnsNil(t *testing.T) {
	mock := &MockAppContext{Ctx: nil}
	assert.Nil(t, mock.Done())
}

func TestMockAppContext_NilCtx_ErrReturnsNil(t *testing.T) {
	mock := &MockAppContext{Ctx: nil}
	assert.Nil(t, mock.Err())
}

func TestMockAppContext_NilLogger_LogMethodsNoop(t *testing.T) {
	mock := NewMock() // L 默认为 nil
	assert.NotPanics(t, func() {
		mock.Info("test")
		mock.Debug("test")
		mock.Warn("test")
		mock.Error(errors.New("err"))
	})
}

func TestMockAppContext_NilLogger_LoggerReturnsSelf(t *testing.T) {
	mock := NewMock()
	assert.Equal(t, mock, mock.Logger())
	// Logger 门面调用也不应 panic
	assert.NotPanics(t, func() {
		mock.Logger().Info("test")
	})
}

// ============================================================
// SQLMock 集成
// ============================================================

func TestWithSQLMock_CreatesGORMAndMock(t *testing.T) {
	mock := NewMock(WithSQLMock())
	defer mock.Close()

	assert.NotNil(t, mock.GORM())
	assert.NotNil(t, mock.SQLMock)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSQLMock_ExpectQuery(t *testing.T) {
	mock := NewMock(WithSQLMock())
	defer mock.Close()

	rows := mock.SQLMock.NewRows([]string{"id", "name"}).
		AddRow(1, "alice").
		AddRow(2, "bob")
	mock.SQLMock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(rows)

	// 执行查询
	type User struct {
		ID   int
		Name string
	}
	var users []User
	err := mock.GORM().Raw("SELECT * FROM `users`").Scan(&users).Error
	assert.NoError(t, err)
	assert.Len(t, users, 2)
	assert.Equal(t, "alice", users[0].Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSQLMock_ExpectExec(t *testing.T) {
	mock := NewMock(WithSQLMock())
	defer mock.Close()

	mock.SQLMock.ExpectExec("INSERT INTO `users`").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := mock.GORM().Exec("INSERT INTO `users` (name) VALUES (?)", "alice").Error
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWithSQLMock_UnmetExpectation(t *testing.T) {
	mock := NewMock(WithSQLMock())
	defer mock.Close()

	mock.SQLMock.ExpectQuery("SELECT .* FROM `users`").WillReturnRows(
		mock.SQLMock.NewRows([]string{"id"}),
	)
	// 故意不执行查询

	err := mock.ExpectationsWereMet()
	assert.Error(t, err, "sqlmock should return error")
}

func TestNewSQLMock_Standalone(t *testing.T) {
	gormDB, sqlMock, err := NewSQLMock()
	require.NoError(t, err)
	defer CloseSQLMock(gormDB)

	rows := sqlMock.NewRows([]string{"id"}).AddRow(1)
	sqlMock.ExpectQuery("SELECT").WillReturnRows(rows)

	var result int
	err = gormDB.Raw("SELECT 1 AS id").Scan(&result).Error
	assert.NoError(t, err)
	assert.Equal(t, 1, result)
	assert.NoError(t, sqlMock.ExpectationsWereMet())
}

func TestExpectationsWereMet_NoSQLMock(t *testing.T) {
	mock := NewMock() // 无 SQLMock
	assert.NoError(t, mock.ExpectationsWereMet(), "SQLMock should be nil")
}

func TestClose_NoDB(t *testing.T) {
	mock := NewMock() // 无 DB
	assert.NoError(t, mock.Close())
}
