package e

import (
	"errors"
)

// IsCode 判断错误是否匹配指定错误码
func IsCode(err error, code string) bool {
	if err == nil {
		return code == SuccessCode
	}
	var coder ErrorCoder
	if errors.As(err, &coder) {
		return coder.GetCode() == code
	}
	return false
}

// IsClientError 判断是否为客户端错误
func IsClientError(err error) bool {
	var typed TypedError
	if errors.As(err, &typed) {
		return typed.IsClientError()
	}
	return false
}

// Wrap 包装标准错误为业务错误
func Wrap(err error, code string, messageID string) ServiceError {
	if err == nil {
		return nil
	}
	return New(code, messageID).Wrap(err).Build()
}

// WrapInternal 包装为内部错误
func WrapInternal(err error) ServiceError {
	return Wrap(err, InternalErrorCode, "internal_error")
}

// As 类型转换
func As(err error) (ServiceError, bool) {
	var se ServiceError
	if errors.As(err, &se) {
		return se, true
	}
	return nil, false
}

// Cause 获取最原始错误
func Cause(err error) error {
	for err != nil {
		wrapper, ok := err.(Wrapper)
		if !ok {
			break
		}
		cause := wrapper.Unwrap()
		if cause == nil {
			break
		}
		err = cause
	}
	return err
}
