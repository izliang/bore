package escommon

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/izliang/bore/logprobe"
)

// ==========================================================================
// ExtractSourceInfo — 从 _mapping 响应提取 SourceInfo
// ==========================================================================

// ExtractSourceInfo 从 _mapping 响应中提取字段列表，结合预估文档数构建 SourceInfo。
//
// 如果 _mapping 无权限（返回 {}），请改用 ExtractSourceInfoFromFieldCaps。
//
// 参数：
//   - name: 数据源名称
//   - index: 索引模式
//   - mappingBody: GET /{index}/_mapping 响应
//   - estimated: 预估文档总数
//   - timeField: 时间字段名
//   - msgField: 消息字段名
func ExtractSourceInfo(name, index string, mappingBody io.Reader, estimated int64, timeField, msgField string) (*logprobe.SourceInfo, error) {
	info := &logprobe.SourceInfo{
		Name:         name,
		Backend:      "elasticsearch",
		TimeField:    timeField,
		MessageField: msgField,
		Estimated:    estimated,
	}

	fields, err := extractFieldsFromMapping(mappingBody, timeField, msgField)
	if err != nil {
		return info, nil // mapping 解析失败不致命，返回部分信息
	}
	info.Fields = fields
	return info, nil
}

// ==========================================================================
// ExtractSourceInfoFromFieldCaps — 从 _field_caps 响应提取 SourceInfo
// ==========================================================================

// ExtractSourceInfoFromFieldCaps 从 _field_caps 响应中提取字段列表。
//
// _field_caps API 的优势：
//   - 直接返回 searchable / aggregatable 元数据，无需从 mapping type 推导
//   - 通常比 _mapping 权限更宽松（不需要 monitor 权限）
//
// 使用方式：GET /{index}/_field_caps?fields=*
//
// 响应格式：
//
//	{
//	  "fields": {
//	    "@timestamp": {"date": {"type":"date","searchable":true,"aggregatable":true}},
//	    "message": {"text": {"type":"text","searchable":true,"aggregatable":false}},
//	    "level": {"keyword": {"type":"keyword","searchable":true,"aggregatable":true}}
//	  }
//	}
//
// 参数：
//   - name: 数据源名称
//   - index: 索引模式
//   - fieldCapsBody: GET /{index}/_field_caps?fields=* 响应
//   - estimated: 预估文档总数
//   - timeField: 时间字段名
//   - msgField: 消息字段名
func ExtractSourceInfoFromFieldCaps(name, index string, fieldCapsBody io.Reader, estimated int64, timeField, msgField string) (*logprobe.SourceInfo, error) {
	info := &logprobe.SourceInfo{
		Name:         name,
		Backend:      "elasticsearch",
		TimeField:    timeField,
		MessageField: msgField,
		Estimated:    estimated,
	}

	fields, err := extractFieldsFromFieldCaps(fieldCapsBody, timeField, msgField)
	if err != nil {
		return info, nil
	}
	// 附加上下文过滤条件：字段名包含 "@" 或属于已知日志常用字段时保留
	info.Fields = fields
	return info, nil
}

// ==========================================================================
// 内部解析：_mapping
// ==========================================================================

// extractFieldsFromMapping 从 _mapping 响应中提取字段元数据列表。
func extractFieldsFromMapping(body io.Reader, timeField, msgField string) ([]logprobe.FieldInfo, error) {
	var raw map[string]struct {
		Mappings struct {
			Properties map[string]esProperty `json:"properties"`
		} `json:"mappings"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析 _mapping 失败: %w", err)
	}

	var allFields []logprobe.FieldInfo
	seen := make(map[string]bool)
	for _, idx := range raw {
		for name, prop := range idx.Mappings.Properties {
			fi := mapESProperty(name, prop, timeField, msgField)
			if seen[name] {
				continue
			}
			seen[name] = true
			allFields = append(allFields, fi)
		}
	}
	return allFields, nil
}

// esProperty 表示 ES mapping 中的一个字段定义。
type esProperty struct {
	Type       string                `json:"type"`
	Fields     map[string]esProperty `json:"fields,omitempty"`
	Properties map[string]esProperty `json:"properties,omitempty"`
}

// mapESProperty 将 ES mapping property 映射为 logprobe.FieldInfo。
func mapESProperty(name string, prop esProperty, timeField, msgField string) logprobe.FieldInfo {
	fi := logprobe.FieldInfo{Name: name, Type: mapESType(prop.Type)}

	switch prop.Type {
	case "text", "keyword", "wildcard":
		fi.Searchable = true
	default:
		fi.Searchable = false
	}

	assignRoles(&fi, name, timeField, msgField)
	return fi
}

// ==========================================================================
// 内部解析：_field_caps
// ==========================================================================

// extractFieldsFromFieldCaps 从 _field_caps 响应中提取字段元数据列表。
//
// _field_caps 对每个字段返回所有可用类型（如 text 字段同时有 text 和 keyword 两种 mapping）。
// 我们取 searchable=true 的类型作为主类型；如果都不 searchable，取第一个类型。
func extractFieldsFromFieldCaps(body io.Reader, timeField, msgField string) ([]logprobe.FieldInfo, error) {
	var raw struct {
		Fields map[string]map[string]esFieldCapsEntry `json:"fields"`
	}
	if err := json.NewDecoder(body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析 _field_caps 失败: %w", err)
	}

	// 如果 fields 为空（无权限返回 {}），返回空列表
	if len(raw.Fields) == 0 {
		return nil, nil
	}

	var allFields []logprobe.FieldInfo
	for name, typeMap := range raw.Fields {
		fi := mapFieldCapsEntry(name, typeMap)
		assignRoles(&fi, name, timeField, msgField)
		allFields = append(allFields, fi)
	}
	return allFields, nil
}

// esFieldCapsEntry 表示 _field_caps 中一个字段的单个类型信息。
type esFieldCapsEntry struct {
	Type          string `json:"type"`
	Searchable    bool   `json:"searchable"`
	Aggregatable  bool   `json:"aggregatable"`
	MetadataField bool   `json:"metadata_field"`
}

// mapFieldCapsEntry 将 _field_caps entry 映射为 logprobe.FieldInfo。
//
// 如果一个字段有多个类型（如 text 同时有 text + .keyword），
// 优先选 searchable=true 的作为主类型。如果都 searchable，选 keyword 优先。
func mapFieldCapsEntry(name string, typeMap map[string]esFieldCapsEntry) logprobe.FieldInfo {
	fi := logprobe.FieldInfo{Name: name}

	if len(typeMap) == 0 {
		return fi
	}

	// 策略：
	// 1. 优先取 keyword 类型（精确匹配）
	// 2. 其次取 searchable=true 的类型
	// 3. 最后取第一个类型

	var bestType string
	var bestEntry esFieldCapsEntry

	// 尝试 keyword
	if entry, ok := typeMap["keyword"]; ok {
		bestType = "keyword"
		bestEntry = entry
	} else {
		// 尝试 searchable
		for t, entry := range typeMap {
			if entry.Searchable {
				bestType = t
				bestEntry = entry
				break
			}
		}
	}

	// 如果都没有 keyword 也没有 searchable，取第一个
	if bestType == "" {
		for t, entry := range typeMap {
			bestType = t
			bestEntry = entry
			break
		}
	}

	fi.Type = mapESType(bestType)
	fi.Searchable = bestEntry.Searchable

	// _field_caps 不提供 distinctN，从 aggregatable 推断大致基数
	// aggregatable=true 的字段通常基数不太高
	if bestEntry.Aggregatable {
		fi.DistinctN = -1 // -1 表示"可聚合但基数未知"
	}

	return fi
}

// ==========================================================================
// 共享辅助
// ==========================================================================

// assignRoles 根据字段名自动标注语义角色。
func assignRoles(fi *logprobe.FieldInfo, name, timeField, msgField string) {
	if name == timeField {
		fi.Roles = append(fi.Roles, "timestamp")
	}
	if name == msgField {
		fi.Roles = append(fi.Roles, "message")
	}

	// 常用字段名自动标注
	roleMap := map[string]string{
		"trace_id":     "trace_id",
		"traceId":      "trace_id",
		"trace.id":     "trace_id",
		"level":        "level",
		"log.level":    "level",
		"severity":     "level",
		"host":         "host",
		"host.name":    "host",
		"hostname":     "host",
		"service":      "service",
		"service.name": "service",
	}
	if role, ok := roleMap[name]; ok {
		fi.Roles = append(fi.Roles, role)
	}
}

// mapESType 将 ES 字段类型映射为简化类型标识。
func mapESType(esType string) string {
	switch esType {
	case "keyword", "text", "wildcard", "match_only_text":
		return "string"
	case "long", "integer", "short", "byte", "double", "float", "half_float", "scaled_float":
		return "number"
	case "boolean":
		return "boolean"
	case "date", "date_nanos":
		return "datetime"
	case "ip":
		return "ip"
	case "object", "nested", "flattened":
		return "object"
	default:
		if esType == "" {
			return "object"
		}
		return esType
	}
}
