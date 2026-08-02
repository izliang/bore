package carrier

import "google.golang.org/grpc/metadata"

// GRPCMetadata 将 metadata.MD 适配为 propagator.Carrier。
type GRPCMetadata struct {
	MD metadata.MD
}

func (g *GRPCMetadata) Get(key string) string {
	vals := g.MD.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (g *GRPCMetadata) Set(key string, value string) {
	g.MD.Set(key, value)
}

func (g *GRPCMetadata) Keys() []string {
	keys := make([]string, 0, len(g.MD))
	for k := range g.MD {
		keys = append(keys, k)
	}
	return keys
}
