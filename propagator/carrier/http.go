package carrier

import "net/http"

// HTTPHeader 将 http.Header 适配为 propagator.Carrier。
type HTTPHeader struct {
	Header http.Header
}

func (h *HTTPHeader) Get(key string) string {
	return h.Header.Get(key)
}

func (h *HTTPHeader) Set(key string, value string) {
	h.Header.Set(key, value)
}

func (h *HTTPHeader) Keys() []string {
	keys := make([]string, 0, len(h.Header))
	for k := range h.Header {
		keys = append(keys, k)
	}
	return keys
}
