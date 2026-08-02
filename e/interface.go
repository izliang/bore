package e

import "fmt"

// ErrorCoder 错误码能力接口
type ErrorCoder interface {
	GetCode() string
}

// Messager 错误消息能力接口
type Messager interface {
	GetMessage() string // 返回 i18n message ID
}

// Detailer 错误详情能力接口
type Detailer interface {
	GetData() interface{}
	GetDetails() interface{}
}

// TemplateSupport 支持模板变量的错误
type TemplateSupport interface {
	TemplateData() map[string]interface{}
}

// TypedError 类型化错误接口 — 按 HTTP 语义分类错误
type TypedError interface {
	IsClientError() bool
	IsServerError() bool
	IsNotFound() bool
	IsAuthError() bool
}

// Wrapper 错误包装接口（Go 1.13+ error chain）
type Wrapper interface {
	Unwrap() error
}

// Localizable 可本地化的完整错误接口
type Localizable interface {
	error
	ErrorCoder
	Messager
	Detailer
	TypedError
	fmt.Stringer
}

// ServiceError 服务间传递的错误接口
type ServiceError interface {
	Localizable
	TemplateSupport
	Wrapper
}
