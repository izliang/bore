package amis

import (
	"fmt"
	"html/template"
	"strings"
)

// ResponseAdapter 定义 API 响应到 amis 格式的映射规则。
//
// amis 前端期望的响应格式为：
//
//	{"status": 0, "msg": "", "data": {...}}
//
// 其中 status=0 表示成功，非 0 表示错误。
//
// 多数后端框架使用不同的字段名（如 code/errno/retCode），
// ResponseAdapter 将这些字段映射到 amis 标准格式。
//
// 零值不可用，请使用 DefaultResponseAdapter() 或自定义。
type ResponseAdapter struct {
	// CodeField 响应中表示状态码的字段名。
	// 默认 "code"，对应信封模式：{"code":"000000","message":"...","data":{...}}
	CodeField string

	// MessageField 响应中表示提示消息的字段名。
	// 默认 "message"。
	MessageField string

	// DataField 响应中表示业务数据的字段名。
	// 默认 "data"。
	DataField string

	// SuccessValue 表示成功的状态码值（字符串比较）。
	// 默认 "000000"，常见取值："0"、"200"、"ok"。
	SuccessValue string
}

// DefaultResponseAdapter 返回默认的信封模式适配器。
//
// 适配以下后端响应格式：
//
//	{"code":"000000","message":"success","data":{...}}
//	{"code":"000001","message":"参数错误","data":null}
//
// 映射为 amis 格式：
//
//	{"status":0,"msg":"success","data":{...}}
//	{"status":1,"msg":"参数错误","data":null}
func DefaultResponseAdapter() ResponseAdapter {
	return ResponseAdapter{
		CodeField:    "code",
		MessageField: "message",
		DataField:    "data",
		SuccessValue: "000000",
	}
}

// normalize 填充空字段为默认值。
func (a *ResponseAdapter) normalize() {
	if a.CodeField == "" {
		a.CodeField = "code"
	}
	if a.MessageField == "" {
		a.MessageField = "message"
	}
	if a.DataField == "" {
		a.DataField = "data"
	}
	if a.SuccessValue == "" {
		a.SuccessValue = "000000"
	}
}

// ToJS 生成 JavaScript 适配器函数源码，注入页面 <script>。
//
// 生成的函数签名：
//
//	function adaptAmisResponse(payload) { ... }
//
// 逻辑：
//  1. 非对象直接返回
//  2. 如果 payload 的 CodeField 是字符串 → 映射为 amis 格式
//  3. 如果 payload.status 已经是数字 → 透传（已是 amis 格式）
//  4. 其他情况 → 原样返回
func (a *ResponseAdapter) ToJS() template.JS {
	a.normalize()

	// 构建 JS 代码 —— 字段名使用 JSON 序列化确保安全
	codeField := quoteJS(a.CodeField)
	msgField := quoteJS(a.MessageField)
	dataField := quoteJS(a.DataField)
	successVal := quoteJS(a.SuccessValue)

	js := fmt.Sprintf(`function adaptAmisResponse(payload) {
	console.log("进入",payload)
    if (payload === null || typeof payload !== 'object') return payload;
    var v = payload[%s];
    if (typeof v === 'string' || typeof v === 'number') {
        let ret = {
            status: String(v) === %s ? 0 : (parseInt(v, 10) || 500),
            msg: payload[%s] || '',
            data: payload[%s] !== undefined ? payload[%s] : null
        };
		console.log("返回", ret)
		return ret;
    }
    if (typeof payload.status === 'number') return payload;
	console.log("返回",payload)
    return payload;
}`,
		codeField, successVal,
		msgField,
		dataField, dataField,
	)

	return template.JS(strings.TrimSpace(js))
}

// quoteJS 返回 JSON 编码后的 JS 字符串字面量。
func quoteJS(s string) string {
	// 简单策略：用双引号包裹，转义内部双引号和反斜杠
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	return `"` + escaped + `"`
}
