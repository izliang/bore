package e

// ErrorBuilder 错误构建器
type ErrorBuilder struct {
	code         string
	messageID    string
	errType      string
	data         interface{}
	details      interface{}
	cause        error
	templateData map[string]interface{}
}

// New 创建错误构建器
func New(code string, messageID string) *ErrorBuilder {
	return &ErrorBuilder{
		code:      code,
		messageID: messageID,
		errType:   "business",
	}
}

// FromErrorDef 从错误定义创建构建器
func FromErrorDef(def ErrorDef) *ErrorBuilder {
	return New(def.Code, def.MessageID)
}

// WithType 设置错误类型
func (b *ErrorBuilder) WithType(errType string) *ErrorBuilder {
	b.errType = errType
	return b
}

// WithData 设置附加数据
func (b *ErrorBuilder) WithData(data interface{}) *ErrorBuilder {
	b.data = data
	return b
}

// WithDetails 设置详情
func (b *ErrorBuilder) WithDetails(details interface{}) *ErrorBuilder {
	b.details = details
	return b
}

// Wrap 包装原始错误
func (b *ErrorBuilder) Wrap(cause error) *ErrorBuilder {
	b.cause = cause
	return b
}

// WithTemplate 添加模板变量
func (b *ErrorBuilder) WithTemplate(key string, value interface{}) *ErrorBuilder {
	if b.templateData == nil {
		b.templateData = make(map[string]interface{})
	}
	b.templateData[key] = value
	return b
}

// Build 构建错误
func (b *ErrorBuilder) Build() ServiceError {
	return &AppError{
		Code:         b.code,
		Message:      b.messageID,
		Type:         b.errType,
		Data:         b.data,
		Details:      b.details,
		cause:        b.cause,
		templateData: b.templateData,
	}
}

// InternalError 内部错误
func InternalError(messageID string) *ErrorBuilder {
	return New(InternalErrorCode, messageID).WithType("internal")
}
