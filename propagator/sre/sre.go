package sre

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/izliang/bore/propagator"
)

const (
	sreEnvHeader        = "x-sre-env"
	brhGrayHeaderPrefix = "brh-gray-"
	brhSubHeaderPrefix  = "brh-subversion-"
	brhGrayApiCode      = "brh-gray-apicode"
)

// ──────────────────────────── Context Key ────────────────────────────

type srePropagatorCtxKey struct{}

// SREPropagatorCtxKey 是 SRE 传播数据在 context 中的 key。
var SREPropagatorCtxKey = srePropagatorCtxKey{}

// ──────────────────────────── 数据结构 ──────────────────────────────

// SREPropagatorItem 表示单个服务的 SRE 灰度信息。
type SREPropagatorItem struct {
	ServerName     string `json:"serverName"`
	BrhGrayVersion string `json:"brhGrayVersion"`
	BrhSubVersion  string `json:"brhSubVersion"`
}

// SREPropagatorData 保存 SRE 传播的全部数据。
type SREPropagatorData struct {
	Env     string              `json:"env"`
	ApiCode string              `json:"apiCode"`
	Apps    []SREPropagatorItem `json:"apps"`
	Extra   map[string]string   `json:"extra,omitempty"`
}

// ──────────────────────────── Propagator ─────────────────────────────

var _ propagator.Propagator = (*SREPropagator)(nil)

// SREPropagator 在 HTTP、gRPC、MQ 链路中透传 SRE 灰度、环境、版本及自定义 header。
type SREPropagator struct {
	extraKeys     []string
	extraDefaults map[string]string
}

// SREPropagatorOptFunc 是 SREPropagator 的配置函数。
type SREPropagatorOptFunc func(*SREPropagator)

// WithKey 指定需要从 carrier 中提取并继续透传的额外 key。
func WithKey(key string) SREPropagatorOptFunc {
	return func(p *SREPropagator) {
		p.addExtraKey(key)
	}
}

// WithMap 指定额外 key 的默认透传值，incoming carrier 中的同名值优先。
func WithMap(key, value string) SREPropagatorOptFunc {
	return func(p *SREPropagator) {
		if key == "" {
			return
		}
		p.addExtraKey(key)
		p.extraDefaults[key] = value
	}
}

// New 创建一个 SREPropagator。
func New(opts ...SREPropagatorOptFunc) *SREPropagator {
	s := &SREPropagator{extraDefaults: make(map[string]string)}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Inject 实现 propagator.Propagator。
func (p *SREPropagator) Inject(ctx context.Context, carrier propagator.Carrier) {
	data := Get(ctx)
	if data == nil {
		return
	}
	if data.Env != "" {
		carrier.Set(sreEnvHeader, data.Env)
	}
	if data.ApiCode != "" {
		carrier.Set(brhGrayApiCode, data.ApiCode)
	}
	for _, app := range data.Apps {
		if app.ServerName == "" {
			continue
		}
		if app.BrhGrayVersion != "" {
			carrier.Set(fmt.Sprintf("%s%s", brhGrayHeaderPrefix, app.ServerName), app.BrhGrayVersion)
		}
		if app.BrhSubVersion != "" {
			carrier.Set(fmt.Sprintf("%s%s", brhSubHeaderPrefix, app.ServerName), app.BrhSubVersion)
		}
	}
	for _, key := range p.extraKeys {
		value := p.extraDefaults[key]
		if data.Extra != nil {
			if extracted, ok := data.Extra[key]; ok {
				value = extracted
			} else {
				for extractedKey, extractedValue := range data.Extra {
					if strings.EqualFold(extractedKey, key) {
						value = extractedValue
						break
					}
				}
			}
		}
		if value != "" {
			carrier.Set(key, value)
		}
	}
}

// Extract 实现 propagator.Propagator。
func (p *SREPropagator) Extract(ctx context.Context, carrier propagator.Carrier) context.Context {
	data := p.extractSREHeaders(carrier)
	return Set(ctx, data)
}

// ──────────────────────────── Context 存取 ───────────────────────────

// Set 将 SRE 传播数据写入 context。
func Set(ctx context.Context, data SREPropagatorData) context.Context {
	return context.WithValue(ctx, SREPropagatorCtxKey, &data)
}

// Get 从 context 中读取 SRE 传播数据。
func Get(ctx context.Context) *SREPropagatorData {
	if v, ok := ctx.Value(SREPropagatorCtxKey).(*SREPropagatorData); ok {
		return v
	}
	return nil
}

// ──────────────────────────── 内部逻辑 ───────────────────────────────

func (p *SREPropagator) extractSREHeaders(carrier propagator.Carrier) SREPropagatorData {
	data := SREPropagatorData{Apps: make([]SREPropagatorItem, 0)}
	appMap := make(map[string]*SREPropagatorItem)

	for _, key := range carrier.Keys() {
		value := carrier.Get(key)
		lowerKey := strings.ToLower(key)
		switch {
		case lowerKey == sreEnvHeader:
			if data.Env == "" {
				data.Env = value
			}
		case lowerKey == brhGrayApiCode:
			if data.ApiCode == "" {
				data.ApiCode = value
			}
		case strings.HasPrefix(lowerKey, brhGrayHeaderPrefix):
			serverName := strings.TrimPrefix(lowerKey, brhGrayHeaderPrefix)
			if serverName == "" {
				continue
			}
			item := getOrCreateItem(appMap, serverName)
			item.BrhGrayVersion = value
		case strings.HasPrefix(lowerKey, brhSubHeaderPrefix):
			serverName := strings.TrimPrefix(lowerKey, brhSubHeaderPrefix)
			if serverName == "" {
				continue
			}
			item := getOrCreateItem(appMap, serverName)
			item.BrhSubVersion = value
		}
	}

	serverNames := make([]string, 0, len(appMap))
	for serverName := range appMap {
		serverNames = append(serverNames, serverName)
	}
	sort.Strings(serverNames)
	for _, serverName := range serverNames {
		data.Apps = append(data.Apps, *appMap[serverName])
	}

	data.Extra = p.extractExtra(carrier.Get)
	return data
}

func (p *SREPropagator) extractExtra(get func(key string) string) map[string]string {
	if len(p.extraKeys) == 0 {
		return nil
	}
	extra := make(map[string]string, len(p.extraKeys))
	for _, key := range p.extraKeys {
		if value := get(key); value != "" {
			extra[key] = value
		}
	}
	if len(extra) == 0 {
		return nil
	}
	return extra
}

func (p *SREPropagator) addExtraKey(key string) {
	if key == "" {
		return
	}
	if p.extraDefaults == nil {
		p.extraDefaults = make(map[string]string)
	}
	if sort.SearchStrings(p.extraKeys, key) < len(p.extraKeys) && p.extraKeys[sort.SearchStrings(p.extraKeys, key)] == key {
		return
	}
	p.extraKeys = append(p.extraKeys, key)
	sort.Strings(p.extraKeys)
}

func getOrCreateItem(appMap map[string]*SREPropagatorItem, serverName string) *SREPropagatorItem {
	if item, ok := appMap[serverName]; ok {
		return item
	}
	item := &SREPropagatorItem{ServerName: serverName}
	appMap[serverName] = item
	return item
}
