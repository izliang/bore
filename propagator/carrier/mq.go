package carrier

import "strings"

// MQHeaders 将 map[string]string 适配为 propagator.Carrier。
type MQHeaders struct {
	Headers map[string]string
}

// Get 执行大小写不敏感的 key 查找，与 HTTP Header 行为一致。
func (m *MQHeaders) Get(key string) string {
	if v, ok := m.Headers[key]; ok {
		return v
	}
	for k, v := range m.Headers {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func (m *MQHeaders) Set(key string, value string) {
	m.Headers[key] = value
}

func (m *MQHeaders) Keys() []string {
	keys := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		keys = append(keys, k)
	}
	return keys
}
