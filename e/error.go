package e

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AppError 基础错误实现
type AppError struct {
	Code         string                 `json:"code"`
	Message      string                 `json:"message"`
	Type         string                 `json:"type,omitempty"`
	Data         interface{}            `json:"data,omitempty"`
	Details      interface{}            `json:"details,omitempty"`
	cause        error                  `` // 原始错误，不序列化
	templateData map[string]interface{} `` // 模板数据，不序列化
}

// Error 实现 error 接口
func (e *AppError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("code: %s, message: %s, cause: %v", e.Code, e.Message, e.cause)
	}
	return fmt.Sprintf("code: %s, message: %s", e.Code, e.Message)
}

// String 字符串表示
func (e *AppError) String() string {
	return e.Error()
}

// GetCode 实现 ErrorCoder 接口
func (e *AppError) GetCode() string {
	return e.Code
}

// GetMessage 实现 Messager 接口
func (e *AppError) GetMessage() string {
	return e.Message
}

// GetData 实现 Detailer 接口
func (e *AppError) GetData() interface{} {
	return e.Data
}

// GetDetails 实现 Detailer 接口
func (e *AppError) GetDetails() interface{} {
	return e.Details
}

// TemplateData 实现 TemplateSupport 接口
func (e *AppError) TemplateData() map[string]interface{} {
	return e.templateData
}

// Unwrap 实现 Wrapper 接口
func (e *AppError) Unwrap() error {
	return e.cause
}

// IsClientError 实现 TypedError 接口
func (e *AppError) IsClientError() bool {
	return strings.HasPrefix(e.Code, "4")
}

// IsServerError 实现 TypedError 接口
func (e *AppError) IsServerError() bool {
	return strings.HasPrefix(e.Code, "5")
}

// IsNotFound 实现 TypedError 接口
func (e *AppError) IsNotFound() bool {
	return strings.HasPrefix(e.Code, "404")
}

// IsAuthError 实现 TypedError 接口
func (e *AppError) IsAuthError() bool {
	return strings.HasPrefix(e.Code, "403")
}

// WithData 链式设置数据
func (e *AppError) WithData(data interface{}) *AppError {
	e.Data = data
	return e
}

// WithDetails 链式设置详情
func (e *AppError) WithDetails(details interface{}) *AppError {
	e.Details = details
	return e
}

// Wrap 链式包装原始错误
func (e *AppError) Wrap(cause error) *AppError {
	e.cause = cause
	return e
}

// WithTemplate 链式设置模板变量
func (e *AppError) WithTemplate(key string, value interface{}) *AppError {
	if e.templateData == nil {
		e.templateData = make(map[string]interface{})
	}
	e.templateData[key] = value
	return e
}

// JSON JSON 序列化
func (e *AppError) JSON() string {
	b, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(b)
}
