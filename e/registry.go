package e

import (
	"fmt"
	"sync"
)

// ErrorDef 错误定义
type ErrorDef struct {
	Code        string `json:"code"`
	MessageID   string `json:"message_id"`
	Module      string `json:"module"`
	ModuleName  string `json:"module_name"`
	Description string `json:"description"`
}

// ErrorRegistry 错误注册表
type ErrorRegistry struct {
	mu      sync.RWMutex
	errors  map[string]ErrorDef
	modules map[string]string // 模块号 -> 模块名
}

var defaultRegistry = &ErrorRegistry{
	errors:  make(map[string]ErrorDef),
	modules: make(map[string]string),
}

// RegisterModule 注册模块
func RegisterModule(moduleCode string, moduleName string) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()
	defaultRegistry.modules[moduleCode] = moduleName
}

// RegisterError 注册错误定义
func RegisterError(code, messageID, module, desc string) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()

	moduleName := defaultRegistry.modules[module]
	defaultRegistry.errors[code] = ErrorDef{
		Code:        code,
		MessageID:   messageID,
		Module:      module,
		ModuleName:  moduleName,
		Description: desc,
	}
}

// RegisterErrors 批量注册错误
func RegisterErrors(errors []ErrorDef) {
	defaultRegistry.mu.Lock()
	defer defaultRegistry.mu.Unlock()

	for _, err := range errors {
		err.ModuleName = defaultRegistry.modules[err.Module]
		defaultRegistry.errors[err.Code] = err
	}
}

// GetErrorDef 获取错误定义
func GetErrorDef(code string) (ErrorDef, bool) {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	def, ok := defaultRegistry.errors[code]
	return def, ok
}

// GetModuleName 获取模块名
func GetModuleName(moduleCode string) string {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()
	return defaultRegistry.modules[moduleCode]
}

// ListAllErrors 列出所有错误定义
func ListAllErrors() []ErrorDef {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	result := make([]ErrorDef, 0, len(defaultRegistry.errors))
	for _, def := range defaultRegistry.errors {
		result = append(result, def)
	}
	return result
}

// ListModuleErrors 列出模块错误
func ListModuleErrors(moduleCode string) []ErrorDef {
	defaultRegistry.mu.RLock()
	defer defaultRegistry.mu.RUnlock()

	var result []ErrorDef
	for _, def := range defaultRegistry.errors {
		if def.Module == moduleCode {
			result = append(result, def)
		}
	}
	return result
}

// NewByCode 通过错误码快速构建错误
func NewByCode(code string) (*ErrorBuilder, error) {
	def, ok := GetErrorDef(code)
	if !ok {
		return nil, fmt.Errorf("error code %s not registered", code)
	}
	return FromErrorDef(def), nil
}

// MustNewByCode 通过错误码快速构建错误，失败则 panic
func MustNewByCode(code string) *ErrorBuilder {
	builder, err := NewByCode(code)
	if err != nil {
		panic(err)
	}
	return builder
}
