package amis

import (
	"fmt"
	"strings"
)

// AssetProvider 定义 amis 静态资源（CSS/JS）的提供者接口。
//
// 通过实现此接口，可以灵活替换 CDN 版本或使用本地静态资源，
// 满足内网隔离、私有化部署等场景需求。
type AssetProvider interface {
	// Version 返回 amis 版本号（如 "6.10.0" 或 "latest"）。
	Version() string

	// CSSURLs 返回需要加载的 CSS 资源 URL 列表。
	CSSURLs() []string

	// JSURLs 返回需要加载的 JS 资源 URL 列表。
	JSURLs() []string
}

// Theme 定义 amis 主题类型。
type Theme string

const (
	// ThemeCXD 默认主题（原 cffi），适合企业后台。
	ThemeCXD Theme = "cxd"

	// ThemeDark 暗色主题。
	ThemeDark Theme = "dark"

	// ThemeAntd Ant Design 风格主题。
	ThemeAntd Theme = "antd"
)

// CDN 默认基地址常量。
const (
	// DefaultCDNBase 默认使用 unpkg CDN。
	DefaultCDNBase = "https://unpkg.com"

	// DefaultVersion 未指定版本时使用 latest。
	DefaultVersion = "latest"
)

// CDNAssets 基于 CDN 的内置 AssetProvider 实现。
//
// 使用 unpkg（或其他兼容 CDN）加载 amis SDK 资源。
// SDK 模式已内嵌 React 依赖，无需额外引入。
type CDNAssets struct {
	version string
	theme   Theme
	baseURL string
}

// NewCDNAssets 创建 CDN 资源提供者。
//
// version: amis 版本号（如 "6.10.0"），传空字符串表示使用 latest。
// theme: 主题（ThemeCXD / ThemeDark / ThemeAntd）。
//
// 示例：
//
//	assets := amis.NewCDNAssets("6.10.0", amis.ThemeCXD)
func NewCDNAssets(version string, theme Theme) *CDNAssets {
	if version == "" {
		version = DefaultVersion
	}
	return &CDNAssets{
		version: version,
		theme:   theme,
		baseURL: DefaultCDNBase,
	}
}

// WithBaseURL 设置自定义 CDN 基地址，用于替换默认的 unpkg。
//
// 适用于使用私有 npm 镜像或 cdnjs 等场景：
//
//	assets := amis.NewCDNAssets("6.10.0", amis.ThemeCXD).
//	    WithBaseURL("https://cdnjs.cloudflare.com/ajax/libs")
func (a *CDNAssets) WithBaseURL(baseURL string) *CDNAssets {
	a.baseURL = strings.TrimRight(baseURL, "/")
	return a
}

// Version 返回 amis 版本号。
func (a *CDNAssets) Version() string { return a.version }

// CSSURLs 返回 CSS 资源 URL 列表。
//
// 始终包含 SDK 核心 CSS；当主题非默认时，追加对应的主题 CSS。
func (a *CDNAssets) CSSURLs() []string {
	pkgName := a.pkgName()
	urls := []string{
		fmt.Sprintf("%s/%s/sdk/sdk.css", a.baseURL, pkgName),
		fmt.Sprintf("%s/%s/sdk/helper.css", a.baseURL, pkgName),
		fmt.Sprintf("%s/%s/sdk/iconfont.css", a.baseURL, pkgName),
	}
	// 非 CXD 主题需要加载额外 CSS
	if a.theme != ThemeCXD {
		urls = append(urls,
			fmt.Sprintf("%s/%s/sdk/%s.css", a.baseURL, pkgName, a.theme),
		)
	}
	return urls
}

// JSURLs 返回 JS 资源 URL 列表。
//
// SDK 模式内嵌 React，只需引入 sdk.js。
func (a *CDNAssets) JSURLs() []string {
	return []string{
		fmt.Sprintf("%s/%s/sdk/sdk.js", a.baseURL, a.pkgName()),
	}
}

// pkgName 返回 unpkg 格式的包名（含版本号）。
// "latest" 版本不追加 @ 后缀。
func (a *CDNAssets) pkgName() string {
	if a.version == DefaultVersion {
		return "amis"
	}
	return fmt.Sprintf("amis@%s", a.version)
}
