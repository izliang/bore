package e

const (
	SuccessCode = "000000"
)

const (
	InternalErrorCode = "500000"
)

// 预定义通用错误实例
var (
	Success = &AppError{
		Code:    SuccessCode,
		Message: "success",
		Type:    "success",
	}
	InternalErr = &AppError{
		Code:    InternalErrorCode,
		Message: "internal_error",
		Type:    "internal",
	}
)
