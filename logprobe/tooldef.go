package logprobe

// ==========================================================================
// AI function-calling 工具定义
// ==========================================================================

// ToolDefinition 镜像 OpenAI function-calling 的工具 schema。
// AI 宿主（如 Codex、LangChain、自研 Agent 框架）可以直接注册这些 tool，
// 无需手写 JSON Schema，由 logprobe 包在运行时生成。
type ToolDefinition struct {
	// Type 始终为 "function"（OpenAI 规范）。
	Type string `json:"type"`

	// Function 单个函数的名称、描述和参数 schema。
	Function FunctionDef `json:"function"`
}

// FunctionDef 工具中的一个函数定义。
// Parameters 是 JSON Schema 对象，在运行时由各 params 函数生成。
type FunctionDef struct {
	// Name 函数名，如 "logprobe_search"。
	Name string `json:"name"`

	// Description 函数描述。AI 模型用此字段理解函数的用途和调用时机。
	Description string `json:"description"`

	// Parameters JSON Schema 对象，描述函数的参数结构。
	Parameters any `json:"parameters"`
}

// OpenAITools 返回所有 logprobe 原语的 OpenAI function-calling 工具定义。
//
// AI 宿主在初始化时调用一次，将这些定义注册到 LLM 的 tool registry 中。
// 返回 7 个工具：
//   - logprobe_describe  → Info()     — 发现数据源及其字段结构
//   - logprobe_search    → Search()   — 关键词搜索
//   - logprobe_stats     → Stats()    — 字段分布统计
//   - logprobe_context   → Context()  — 获取日志上下文
//   - logprobe_stream    → Stream()   — 游标流式消费
//   - logprobe_expand    → Expand()   — 展开截断字段的完整内容
//   - logprobe_trace     → Trace()    — 跨数据源分布式追踪
func OpenAITools() []ToolDefinition {
	return []ToolDefinition{
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_describe",
				Description: "Describe available log sources and their schema. Call this first to understand what log sources exist, their fields, and time ranges.",
				Parameters:  describeParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_search",
				Description: "Search log entries by keyword. Returns matching entries with metadata including total hits. If IsExhaustive is false, refine the query (add time range, narrow keyword, or use logprobe_stats). Long fields may be truncated; use logprobe_expand to retrieve full content.",
				Parameters:  searchParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_stats",
				Description: "Get the Top N value distribution for a single field. Use after a broad search to identify promising filter dimensions (e.g. which host has the most errors).",
				Parameters:  statsParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_context",
				Description: "Fetch log entries before and after a target entry. Use to understand what happened around an error. Scope: 'time' (temporal neighbours), 'trace' (same trace/request), or 'host' (same host).",
				Parameters:  contextParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_stream",
				Description: "Start a streaming log iterator. Returns a cursor that can be polled for new entries. Use Follow=true for real-time monitoring, Follow=false to just consume existing data. Save the cursor for checkpoint recovery.",
				Parameters:  streamParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_expand",
				Description: "Expand truncated log fields to their full values. Accepts multiple (entry_id, field) pairs for batch efficiency. Use when a field shows truncated=true.",
				Parameters:  expandParams(),
			},
		},
		{
			Type: "function",
			Function: FunctionDef{
				Name:        "logprobe_trace",
				Description: "Perform a distributed trace across all registered log sources. Given a trace_id, finds all related log entries across sources and returns them sorted by timestamp. Use to trace a request end-to-end through multiple services.",
				Parameters:  traceParams(),
			},
		},
	}
}

// describeParams 返回 logprobe_describe 的 JSON Schema 参数定义。
func describeParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "Optional. ID of a specific source to describe. Omit to list all registered sources.",
			},
		},
		"required": []string{},
	}
}

// searchParams 返回 logprobe_search 的 JSON Schema 参数定义。
func searchParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "ID of the log source to search (from logprobe_describe).",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Keyword or query string. Use '*' for match-all.",
			},
			"time_from": map[string]any{
				"type":        "string",
				"description": "ISO 8601 start time. Omit for unbounded.",
			},
			"time_to": map[string]any{
				"type":        "string",
				"description": "ISO 8601 end time. Omit for unbounded.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max entries to return. Default is backend-specific.",
			},
			"field_max_len": map[string]any{
				"type":        "integer",
				"description": "Max characters per field before truncation. Default 500. Set higher for detailed analysis, lower for scanning.",
			},
			"fields": map[string]any{
				"type":        "array",
				"description": "Specific fields to return. Empty = all fields.",
				"items":       map[string]any{"type": "string"},
			},
		},
		"required": []string{"source_id", "query"},
	}
}

// statsParams 返回 logprobe_stats 的 JSON Schema 参数定义。
func statsParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "ID of the log source.",
			},
			"field": map[string]any{
				"type":        "string",
				"description": "Field to aggregate (e.g. 'level', 'host', 'service'). Use field names from logprobe_describe.",
			},
			"top_n": map[string]any{
				"type":        "integer",
				"description": "Number of top values to return. Default 20.",
			},
			"filter_query": map[string]any{
				"type":        "string",
				"description": "Optional filter to narrow the stats scope (same semantics as search query).",
			},
		},
		"required": []string{"source_id", "field"},
	}
}

// contextParams 返回 logprobe_context 的 JSON Schema 参数定义。
func contextParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "ID of the log source.",
			},
			"entry_id": map[string]any{
				"type":        "string",
				"description": "The reference log entry ID (from a previous search result).",
			},
			"before": map[string]any{
				"type":        "integer",
				"description": "Number of entries to fetch before the reference. Default 20.",
			},
			"after": map[string]any{
				"type":        "integer",
				"description": "Number of entries to fetch after the reference. Default 20.",
			},
			"scope": map[string]any{
				"type":        "string",
				"description": "Ordering dimension: 'time' (temporal neighbours), 'trace' (same trace_id), or 'host' (same host).",
				"enum":        []string{"time", "trace", "host"},
			},
			"scope_field": map[string]any{
				"type":        "string",
				"description": "Field name for trace/host scope (e.g. 'trace_id', 'host.name'). Required when scope is 'trace' or 'host'.",
			},
		},
		"required": []string{"source_id", "entry_id", "scope"},
	}
}

// streamParams 返回 logprobe_stream 的 JSON Schema 参数定义。
func streamParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "ID of the log source.",
			},
			"query": map[string]any{
				"type":        "string",
				"description": "Keyword filter. Use '*' for all logs.",
			},
			"follow": map[string]any{
				"type":        "boolean",
				"description": "Whether to keep streaming new data after existing data is consumed. true for real-time monitoring.",
			},
			"time_from": map[string]any{
				"type":        "string",
				"description": "ISO 8601 start time.",
			},
		},
		"required": []string{"source_id", "query", "follow"},
	}
}

// expandParams 返回 logprobe_expand 的 JSON Schema 参数定义。
func expandParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"source_id": map[string]any{
				"type":        "string",
				"description": "ID of the log source.",
			},
			"targets": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"entry_id": map[string]any{"type": "string", "description": "Log entry ID from a previous search result."},
						"field":    map[string]any{"type": "string", "description": "Field name to expand (e.g. 'stack_trace', 'message')."},
					},
					"required": []string{"entry_id", "field"},
				},
				"description": "List of (entry_id, field) pairs to expand.",
			},
		},
		"required": []string{"source_id", "targets"},
	}
}

// traceParams 返回 logprobe_trace 的 JSON Schema 参数定义。
func traceParams() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"trace_id": map[string]any{
				"type":        "string",
				"description": "Distributed trace identifier.",
			},
			"sources": map[string]any{
				"type":        "array",
				"items":       map[string]any{"type": "string"},
				"description": "Specific source IDs to search. Omit to auto-discover all sources that have a trace_id field.",
			},
			"max_spans": map[string]any{
				"type":        "integer",
				"description": "Maximum total spans to return. 0 = no limit.",
			},
		},
		"required": []string{"trace_id"},
	}
}
