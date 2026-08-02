package core

import "fmt"

// Option 表示类型 T 的类型安全可选值。
// 在 SDK 中广泛用于表示值的存在或缺失，避免使用 nil 指针、哨兵值
// 或接口方法中的多返回值签名。
type Option[T any] struct {
	value   T
	present bool
}

// Some 创建一个包含给定值的 Option。
func Some[T any](v T) Option[T] {
	return Option[T]{value: v, present: true}
}

// None 创建一个不包含任何值的空 Option。
func None[T any]() Option[T] {
	return Option[T]{}
}

// Get 返回包含的值和 true（如果存在），
// 如果 Option 为空，则返回 T 的零值和 false。
func (o Option[T]) Get() (T, bool) {
	return o.value, o.present
}

// MustGet 返回包含的值。如果 Option 为空，则触发 panic。
// 仅在调用方已通过 IsPresent 或 Get 验证了值存在时才使用此方法。
func (o Option[T]) MustGet() T {
	if !o.present {
		panic("core: 在空 Option 上调用了 MustGet")
	}
	return o.value
}

// IsPresent 返回 Option 是否包含值。
func (o Option[T]) IsPresent() bool {
	return o.present
}

// OrElse 如果 Option 包含值则返回该值，否则返回 fallback。
func (o Option[T]) OrElse(fallback T) T {
	if o.present {
		return o.value
	}
	return fallback
}

// String 返回 Option 的可读表示。
func (o Option[T]) String() string {
	if o.present {
		return fmt.Sprintf("Some(%v)", o.value)
	}
	return "None"
}
