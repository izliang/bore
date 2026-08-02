package core

import "errors"

// 哨兵错误，覆盖所有数据结构中的常见失败模式。
// ds 中的各包应使用 fmt.Errorf("%w: ...", core.ErrXxx, ...) 包装这些错误，
// 以提供上下文相关的错误信息。
var (
	// ErrEmpty 表示在空集合上执行了需要非空集合的操作。
	ErrEmpty = errors.New("core: 集合为空")

	// ErrNotFound 表示在集合中未找到所请求的元素。
	ErrNotFound = errors.New("core: 元素未找到")

	// ErrInvalidInput 表示操作的输入在语法或语义上无效。
	ErrInvalidInput = errors.New("core: 无效输入")

	// ErrOutOfBounds 表示索引、位置或偏移量超出了集合的有效范围。
	ErrOutOfBounds = errors.New("core: 索引越界")

	// ErrNotSupported 表示当前实现或配置不支持所请求的操作。
	ErrNotSupported = errors.New("core: 不支持的操作")
)
