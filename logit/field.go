package logit

import (
	"crypto/md5"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	json "github.com/bytedance/sonic"
)

// Field 表示日志中的单条结构化键值对。
// Value 方法返回渲染后的键和值（字符串形式，可直接输出）。
type Field interface {
	Value() (key, value string)
}

// FieldFunc 是一个延迟求值函数，仅在日志行实际输出时才构造 Field。
// 这种延迟避免了日志级别被过滤时仍然进行昂贵的字段构造，从而提升性能。
type FieldFunc func() Field

// ============================================================================
// 泛型字段：统一处理基础数值类型的内部实现
// ============================================================================

// valueField 是泛型字段，为任意类型提供统一的存储和格式化。
// 替代了原有的 intField / uintField / floatField 等多个重复结构体。
type valueField[T any] struct {
	key    string
	val    T
	format func(T) string
}

// Value 返回键及格式化后的值字符串。
func (f *valueField[T]) Value() (string, string) {
	return f.key, f.format(f.val)
}

// FieldOf 创建任意类型的 FieldFunc，需提供自定义格式化函数。
// 这是泛型字段体系的基础构造器。对于常见的数值类型，推荐使用 IntOf / UintOf / FloatOf 便捷方法。
func FieldOf[T any](name string, value T, format func(T) string) FieldFunc {
	f := &valueField[T]{key: name, val: value, format: format}
	return func() Field { return f }
}

// IntOf 为有符号整数创建 FieldFunc，覆盖 int / int8 / int16 / int32 / int64。
func IntOf[T ~int | ~int8 | ~int16 | ~int32 | ~int64](name string, value T) FieldFunc {
	return FieldOf(name, value, func(v T) string { return strconv.FormatInt(int64(v), 10) })
}

// UintOf 为无符号整数创建 FieldFunc，覆盖 uint / uint8 / uint16 / uint32 / uint64。
func UintOf[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](name string, value T) FieldFunc {
	return FieldOf(name, value, func(v T) string { return strconv.FormatUint(uint64(v), 10) })
}

// FloatOf 为浮点数创建 FieldFunc，覆盖 float32 / float64。
func FloatOf[T ~float32 | ~float64](name string, value T) FieldFunc {
	return FieldOf(name, value, func(v T) string { return strconv.FormatFloat(float64(v), 'f', -1, 64) })
}

// ============================================================================
// 兼容旧 API：委托泛型实现，保留原有函数签名
// ============================================================================

// IntField 为 int 创建 FieldFunc（向后兼容，内部委托 IntOf）。
func IntField(name string, value int) FieldFunc { return IntOf(name, value) }

// Int64Field 为 int64 创建 FieldFunc（向后兼容，内部委托 IntOf）。
func Int64Field(name string, value int64) FieldFunc { return IntOf(name, value) }

// UintField 为 uint 创建 FieldFunc（向后兼容，内部委托 UintOf）。
func UintField(name string, value uint) FieldFunc { return UintOf(name, value) }

// Uint64Field 为 uint64 创建 FieldFunc（向后兼容，内部委托 UintOf）。
func Uint64Field(name string, value uint64) FieldFunc { return UintOf(name, value) }

// Float32Field 为 float32 创建 FieldFunc（向后兼容，内部委托 FloatOf）。
func Float32Field(name string, value float32) FieldFunc { return FloatOf(name, value) }

// Float64Field 为 float64 创建 FieldFunc（向后兼容，内部委托 FloatOf）。
func Float64Field(name string, value float64) FieldFunc { return FloatOf(name, value) }

// ============================================================================
// 字符串字段
// ============================================================================

// stringField 持有带换行转义配置的字符串值。
type stringField struct {
	key          string // 字段键名
	val          string // 预渲染的字段值
	allowNewline bool   // 若为 false，则换行符被转义为 \n
}

// stringConfig 持有字符串字段的配置选项。
type stringConfig struct {
	allowNewline bool // 是否在输出中保留换行符
}

// Value 返回键及渲染后的字符串值。
func (sf *stringField) Value() (string, string) {
	return sf.key, sf.val
}

// StrConfigFunc 是字符串字段的配置选项构造器。
type StrConfigFunc func(*stringConfig)

// WithStrAllowNewline 返回一个控制换行符保留或转义的选项。
// 为 false（默认）时，换行符被转义为 \n 以保持单行日志。
func WithStrAllowNewline(allowNewline bool) StrConfigFunc {
	return func(c *stringConfig) {
		c.allowNewline = allowNewline
	}
}

// StringField 为 UTF-8 字符串值创建一个 FieldFunc。
// 默认情况下，换行符被转义以防止多行日志条目。
func StringField(name string, value string, opts ...StrConfigFunc) FieldFunc {
	config := &stringConfig{}
	for _, opt := range opts {
		opt(config)
	}
	if !config.allowNewline {
		value = escapeNewline(value)
	}
	f := &stringField{
		key: name,
		val: value,
	}
	return func() Field {
		return f
	}
}

// ============================================================================
// 布尔字段
// ============================================================================

// boolField 持有布尔值。
type boolField struct {
	key string // 字段键名
	val bool   // 布尔值
}

// Value 返回键及 "true" 或 "false"。
func (f *boolField) Value() (string, string) {
	return f.key, strconv.FormatBool(f.val)
}

// BoolField 为布尔值创建一个 FieldFunc。
func BoolField(name string, value bool) FieldFunc {
	return func() Field {
		return &boolField{
			key: name,
			val: value,
		}
	}
}

// ============================================================================
// 错误字段
// ============================================================================

// errorField 持有错误值。
type errorField struct {
	key string // 字段键名
	err error  // 错误（可能为 nil）
}

// Value 返回键及错误消息，若 error 为 nil 则返回 "<nil>"。
func (f *errorField) Value() (string, string) {
	if f.err == nil {
		return f.key, "<nil>"
	}
	return f.key, f.err.Error()
}

// ErrorField 为 error 创建一个 FieldFunc。nil 错误渲染为 "<nil>"。
func ErrorField(name string, err error) FieldFunc {
	return func() Field {
		return &errorField{
			key: name,
			err: err,
		}
	}
}

// ============================================================================
// 任意值字段
// ============================================================================

// anyField 持有一个任意值，将使用 fmt.Sprintf 进行格式化。
type anyField struct {
	key string // 字段键名
	val any    // 待格式化的任意值
}

// Value 返回键及 "%v" 格式化后的字符串。
func (f *anyField) Value() (string, string) {
	return f.key, fmt.Sprintf("%v", f.val)
}

// AnyField 为任意值（使用 "%v" 格式化）创建一个 FieldFunc。
func AnyField(name string, value any) FieldFunc {
	return func() Field {
		return &anyField{
			key: name,
			val: value,
		}
	}
}

// ============================================================================
// 字符串截断字段（StringAbsField）
// ============================================================================

// stringAbsField 持有已做绝对化（截断）处理的字符串值。
type stringAbsField struct {
	key string // 字段键名
	val string // 预处理并截断后的值
}

// Value 返回键及预处理后的字符串值。
func (s *stringAbsField) Value() (string, string) {
	return s.key, s.val
}

// stringAbsConfig 持有绝对字符串截断的配置选项。
// StringAbsField（abs = abstract/absolute）将长字符串截断为固定预览长度，
// 同时保留开头、中间和结尾的内容，并包含完整内容的 MD5 用于完整性校验。
type stringAbsConfig struct {
	head         int                       // 从开头保留的 rune 数
	middle       int                       // 从中间保留的 rune 数
	tail         int                       // 从结尾保留的 rune 数
	format       func(input string) string // 自定义格式化函数（可选）
	allowNewline bool                      // 是否在输出中保留换行符
}

// StringAbsParamFunc 是 StringAbsField 的配置选项构造器。
type StringAbsParamFunc func(input *stringAbsConfig)

// WithStringAbsHead 设置从字符串开头保留的 rune 数。
func WithStringAbsHead(head int) StringAbsParamFunc {
	return func(input *stringAbsConfig) {
		input.head = head
	}
}

// WithAllowNewline 控制换行符是保留（true）还是转义（false）。
func WithAllowNewline(allowNewLine bool) StringAbsParamFunc {
	return func(input *stringAbsConfig) {
		input.allowNewline = allowNewLine
	}
}

// WithStringAbsMiddle 设置从字符串中间保留的 rune 数。
func WithStringAbsMiddle(middle int) StringAbsParamFunc {
	return func(input *stringAbsConfig) {
		input.middle = middle
	}
}

// WithStringAbsTail 设置从字符串结尾保留的 rune 数。
func WithStringAbsTail(tail int) StringAbsParamFunc {
	return func(input *stringAbsConfig) {
		input.tail = tail
	}
}

// WithStringAbsFormatFunc 设置自定义的截断格式化函数。
// 提供后，默认截断逻辑将被跳过。
func WithStringAbsFormatFunc(format func(input string) string) StringAbsParamFunc {
	return func(input *stringAbsConfig) {
		input.format = format
	}
}

// defaultFormat 执行默认的绝对字符串截断。
// 保留 head + middle + tail 个 rune，各段之间插入省略号（...）。
// 输出还包含完整字符串的 rune 总数和 MD5 哈希。
// 格式：<len=N md5=hash preview=head...middle...tail>
func defaultFormat(conf *stringAbsConfig, input string) string {
	if input == "" {
		return ""
	}

	totalRunes := utf8.RuneCountInString(input)

	headN := min(conf.head, totalRunes)
	midN := min(conf.middle, totalRunes-headN)
	tailN := min(conf.tail, totalRunes)

	if headN+midN+tailN >= totalRunes {
		return input
	}

	tailStartRune := totalRunes - tailN
	tailStartRune = max(headN+midN, tailStartRune)

	headEnd := len(input)
	midEnd := len(input)
	tailStart := len(input)

	var i, runeIdx int

	for i < len(input) {
		if runeIdx == headN && headEnd == len(input) {
			headEnd = i
		}
		if runeIdx == headN+midN && midEnd == len(input) {
			midEnd = i
		}
		if runeIdx == tailStartRune {
			tailStart = i
			break
		}
		_, size := utf8.DecodeRuneInString(input[i:])
		i += size
		runeIdx++
	}

	headEnd = min(headEnd, len(input))
	midEnd = min(midEnd, len(input))
	tailStart = min(tailStart, len(input))

	if midEnd < headEnd {
		midEnd = headEnd
	}

	if tailStart < midEnd {
		tailStart = midEnd
	}

	head := input[:headEnd]
	middle := input[headEnd:midEnd]
	tail := input[tailStart:]

	var b strings.Builder
	b.Grow(len(input)/2 + 64)

	b.WriteString("<len=")
	b.WriteString(strconv.Itoa(totalRunes))

	b.WriteString(" md5=")

	sum := md5.Sum([]byte(input))

	const hexTable = "0123456789abcdef"
	for _, v := range sum {
		b.WriteByte(hexTable[v>>4])
		b.WriteByte(hexTable[v&0x0f])
	}

	b.WriteString(" preview=")
	b.WriteString(head)
	b.WriteString("...")
	b.WriteString(middle)
	b.WriteString("...")
	b.WriteString(tail)
	b.WriteByte('>')

	return b.String()
}

// StringAbsField 创建一个 FieldFunc，通过保留开头、中间和结尾的内容来截断可能非常长的字符串，
// 并附带完整内容的 MD5 哈希。适用于记录大型负载（JSON、HTML）而不撑爆日志体积。
//
// 默认：head 保留 200 rune，middle 200 rune，tail 200 rune。
// 使用 WithStringAbsHead、WithStringAbsMiddle、WithStringAbsTail 进行自定义。
func StringAbsField(name string, value string, opts ...StringAbsParamFunc) FieldFunc {
	param := &stringAbsConfig{
		head:   200,
		middle: 200,
		tail:   200,
	}
	for _, opt := range opts {
		opt(param)
	}
	if param.format == nil {
		param.format = func(input string) string {
			return defaultFormat(param, value)
		}
	}
	val := param.format(value)
	if !param.allowNewline {
		val = escapeNewline(val)
	}
	return func() Field {
		return &stringAbsField{
			key: name,
			val: val,
		}
	}
}

// escapeNewline 将换行符和回车符替换为 \n 和 \r 转义序列。
// 这在不允许换行时将日志输出保持为单行。
func escapeNewline(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// ============================================================================
// JSON 截断字段（JSONAbsField / JSONDetailAbsField）
// ============================================================================

// jsonAbsField 持有可 JSON 序列化的值，支持可配置的日志截断。
type jsonAbsField struct {
	key  string
	val  any
	conf *jsonAbsConfig
}

// jsonDetailAbsField 是高性能 JSON 字段，采用流式字符串截断。
// 与 jsonAbsField 不同，它直接编码 JSON，无中间拷贝。
type jsonDetailAbsField struct {
	key  string
	val  any
	conf *jsonAbsConfig
}

// jsonAbsConfig 控制格式化 JSON 日志时的 head/middle/tail 截断。
type jsonAbsConfig struct {
	head         int                    // 从开头保留的 rune 数
	middle       int                    // 从中间保留的 rune 数
	tail         int                    // 从结尾保留的 rune 数
	format       func(input any) string // 自定义格式化函数（可选）
	allowNewline bool                   // 是否在输出中保留换行符
}

// JSONAbsParamFunc 是 JSONAbsField 的配置选项构造器。
type JSONAbsParamFunc func(input *jsonAbsConfig)

// WithJSONAbsHead 设置从字符串开头保留的 rune 数。
func WithJSONAbsHead(head int) JSONAbsParamFunc {
	return func(input *jsonAbsConfig) {
		input.head = head
	}
}

// WithJSONAbsAllowNewline 控制换行符是保留（true）还是转义（false）。
func WithJSONAbsAllowNewline(allowNewLine bool) JSONAbsParamFunc {
	return func(input *jsonAbsConfig) {
		input.allowNewline = allowNewLine
	}
}

// WithJSONAbsMiddle 设置从字符串中间保留的 rune 数。
func WithJSONAbsMiddle(middle int) JSONAbsParamFunc {
	return func(input *jsonAbsConfig) {
		input.middle = middle
	}
}

// WithJSONAbsTail 设置从字符串结尾保留的 rune 数。
func WithJSONAbsTail(tail int) JSONAbsParamFunc {
	return func(input *jsonAbsConfig) {
		input.tail = tail
	}
}

// WithJSONAbsFormatFunc 设置自定义的格式化函数。
// 提供后，默认截断逻辑将被跳过。
func WithJSONAbsFormatFunc(format func(input any) string) JSONAbsParamFunc {
	return func(input *jsonAbsConfig) {
		input.format = format
	}
}

// defaultJSONAbsFormat 通过 processValue 递归截断长字符串后将 v 序列化为 JSON；
// 默认使用紧凑 JSON，除非 allowNewline 请求缩进输出。
func defaultJSONAbsFormat(conf *jsonAbsConfig, v any) string {
	sac := &stringAbsConfig{
		head:         conf.head,
		middle:       conf.middle,
		tail:         conf.tail,
		format:       nil,
		allowNewline: conf.allowNewline,
	}
	val := processValue(v, sac)
	if conf.allowNewline {
		bs, err := json.MarshalIndent(val, "", "    ")
		if err != nil {
			return ""
		}
		return string(bs)
	}
	bs, err := json.Marshal(val)
	if err != nil {
		return ""
	}
	return string(bs)
}

// processValue 递归遍历 map 和 slice，对字符串和 UTF-8 字节值应用字符串截断规则。
func processValue(v any, conf *stringAbsConfig) any {

	switch val := v.(type) {
	case string:
		return defaultFormat(conf, val)
	case []byte:
		if !utf8.Valid(val) {
			return fmt.Sprintf("<bytes len=%d>", len(val))
		}
		return defaultFormat(
			conf,
			string(val),
		)
	case map[string]any:
		for k, vv := range val {
			val[k] = processValue(vv, conf)
		}
		return val
	case []any:
		for i, vv := range val {
			val[i] = processValue(vv, conf)
		}
		return val
	default:
		return v
	}
}

// normalizeJSON 将 v 通过 JSON 序列化/反序列化往返，使嵌套结构变为可遍历的 map[string]any / []any。
func normalizeJSON(v any) any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err = json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

// Value 返回键及由配置的格式化器生成的格式化 JSON 字符串（默认截断长字符串）。
func (f *jsonAbsField) Value() (string, string) {
	obj := normalizeJSON(f.val)
	if obj == nil {
		return f.key, "{}"
	}
	return f.key, f.conf.format(f.val)
}

// JSONAbsField 为序列化为 JSON 的任意值创建一个 FieldFunc。
// map/slice 内的长字符串使用与 StringAbsField 相同的 head/middle/tail 规则进行截断。
// 默认每段保留 200 rune；使用 WithJSONAbsHead、WithJSONAbsMiddle、WithJSONAbsTail 覆盖。
// 使用 WithJSONAbsFormatFunc 替换整个格式化步骤。
func JSONAbsField(name string, value any, opts ...JSONAbsParamFunc) FieldFunc {
	param := &jsonAbsConfig{
		head:   200,
		middle: 200,
		tail:   200,
	}

	for _, opt := range opts {
		opt(param)
	}
	if param.format == nil {
		param.format = func(input any) string {
			return defaultJSONAbsFormat(param, value)
		}
	}

	return func() Field {
		return &jsonAbsField{
			key:  name,
			val:  value,
			conf: param,
		}
	}
}

// ============================================================================
// 时长字段
// ============================================================================

type TimeFormatMethod int8

const (
	TimeFormatMS          TimeFormatMethod = 0 // 默认按照毫秒维度进行格式化
	TimeFormatAdaptiveStr TimeFormatMethod = 1 // 按照自适应的格式，最小ms，最大H小时。
)

// timeDurationField 记录 time.Duration，采用紧凑的纯 ASCII 渲染（低于 1s 用 ms，然后 s/m/h 分级）。
type timeDurationField struct {
	key    string
	val    time.Duration
	format TimeFormatMethod
}

type TimeDurationFunc func(f *timeDurationField)

func WithTimeDurationFormatMethod(fm TimeFormatMethod) TimeDurationFunc {
	return func(f *timeDurationField) {
		f.format = fm
	}
}

// TimeDurationField 为时长创建一个 FieldFunc；输出格式见 formatDurationFieldValue。
func TimeDurationField(name string, value time.Duration, opts ...TimeDurationFunc) FieldFunc {
	return func() Field {
		f := &timeDurationField{
			key:    name,
			val:    value,
			format: TimeFormatMS,
		}
		for _, opt := range opts {
			opt(f)
		}
		return f
	}
}

// Value 返回键及格式化后的时长字符串
func (f *timeDurationField) Value() (string, string) {
	switch f.format {
	case TimeFormatMS:
		return f.key, strconv.FormatInt(int64(f.val/time.Millisecond), 10)
	case TimeFormatAdaptiveStr:
		return f.key, formatDurationFieldValue(f.val)
	default:
		return f.key, formatDurationFieldValue(f.val)
	}
}

// formatDurationFieldValue 为日志渲染时长：纯 ASCII，低于 1s 最小单位为 ms，
// 然后为 s，再然后为 XmYs / XhYmZs 以便快速扫描（不使用 µs/ns 单位）。
func formatDurationFieldValue(d time.Duration) string {
	if d < 0 {
		return "-" + formatDurationFieldValue(-d)
	}
	if d < time.Second {
		if d == 0 {
			return "0ms"
		}
		return strconv.FormatFloat(float64(d)/float64(time.Millisecond), 'f', -1, 64) + "ms"
	}
	if d < time.Minute {
		return strconv.FormatFloat(float64(d)/float64(time.Second), 'f', -1, 64) + "s"
	}
	if d < time.Hour {
		m := int(d / time.Minute)
		s := float64(d%time.Minute) / float64(time.Second)
		return strconv.Itoa(m) + "m" + strconv.FormatFloat(s, 'f', -1, 64) + "s"
	}
	h := int(d / time.Hour)
	rem := d % time.Hour
	m := int(rem / time.Minute)
	s := float64(rem%time.Minute) / float64(time.Second)
	return strconv.Itoa(h) + "h" + strconv.Itoa(m) + "m" + strconv.FormatFloat(s, 'f', -1, 64) + "s"
}

// ============================================================================
// 高性能 JSON 流式编码字段（JSONDetailAbsField）
// ============================================================================

// JSONDetailAbsField 是高性能 JSON 字段，采用流式字符串截断。
// 它直接编码 JSON，无需中间归一化或拷贝，仅截断 string 和 []byte 值。
// JSON 结构保持完整。默认每段保留 200 rune；使用 WithJSONAbsHead、WithJSONAbsMiddle、WithJSONAbsTail 覆盖。
func JSONDetailAbsField(name string, value any, opts ...JSONAbsParamFunc) FieldFunc {
	param := &jsonAbsConfig{
		head:   200,
		middle: 200,
		tail:   200,
	}

	for _, opt := range opts {
		opt(param)
	}

	return func() Field {
		return &jsonDetailAbsField{
			key:  name,
			val:  value,
			conf: param,
		}
	}
}

// Value 返回键及使用流式编码的截断 JSON 字符串。
// 此方法性能优异——它在截断长字符串的同时直接构建 JSON。
func (f *jsonDetailAbsField) Value() (string, string) {
	buf := &strings.Builder{}
	buf.Grow(1024)

	obj := normalizeJSON(f.val)
	if obj == nil {
		return f.key, "{}"
	}

	writeJSONValue(buf, obj, f.conf)
	result := buf.String()

	if !f.conf.allowNewline {
		return f.key, result
	}
	return f.key, escapeNewline(result)
}

// writeJSONValue 递归地将 v 写入 buf，同时对 string/[]byte 进行字符串截断。
// 这是快速路径——无中间拷贝，单次编码。
func writeJSONValue(buf *strings.Builder, v any, conf *jsonAbsConfig) {
	if v == nil {
		buf.WriteString("null")
		return
	}

	switch val := v.(type) {
	case string:
		writeJSONString(buf, val, conf)
	case []byte:
		if !utf8.Valid(val) {
			buf.WriteByte('"')
			buf.WriteString(fmt.Sprintf("<bytes len=%d>", len(val)))
			buf.WriteByte('"')
			return
		}
		writeJSONString(buf, string(val), conf)
	case map[string]any:
		buf.WriteByte('{')
		first := true
		for k, vv := range val {
			if !first {
				buf.WriteByte(',')
			}
			first = false
			writeJSONString(buf, k, conf)
			buf.WriteByte(':')
			writeJSONValue(buf, vv, conf)
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, vv := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONValue(buf, vv, conf)
		}
		buf.WriteByte(']')
	default:
		b, _ := json.Marshal(val)
		buf.Write(b)
	}
}

// writeJSONString 写入经截断的 JSON 字符串，并做适当转义。
func writeJSONString(buf *strings.Builder, s string, conf *jsonAbsConfig) {
	sac := &stringAbsConfig{
		head:         conf.head,
		middle:       conf.middle,
		tail:         conf.tail,
		allowNewline: conf.allowNewline,
	}
	truncated := defaultFormat(sac, s)

	buf.WriteByte('"')
	start := 0
	for i := 0; i < len(truncated); {
		c := truncated[i]
		if c >= 0x20 && c != '"' && c != '\\' {
			i++
			continue
		}
		if start < i {
			buf.WriteString(truncated[start:i])
		}
		switch c {
		case '"', '\\':
			buf.WriteByte('\\')
			buf.WriteByte(c)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteByte(c)
		}
		i++
		start = i
	}
	if start < len(truncated) {
		buf.WriteString(truncated[start:])
	}
	buf.WriteByte('"')
}
