package i18n

import (
	"embed"
)

// Config 加载配置。
//
// 至少需要指定一个加载来源（Dir / Dirs / FS）。
// DefaultLang 和 StrictMode 有合理默认值。
type Config struct {
	// ── 加载来源（至少指定一个） ──

	// Dir 单一语言文件目录。
	// 目录下所有 *.json 文件被扫描，按文件名前缀归并语言。
	Dir string

	// Dirs 多个目录，按顺序加载。
	// 后加载的同 key 覆盖先加载的。
	// 适用场景：基础翻译目录 + 项目自定义覆盖目录。
	Dirs []string

	// FS embed 文件系统（与 Dir/Dirs 互斥或组合使用）。
	FS embed.FS

	// SubDir FS 内的子目录路径。仅当使用 FS 时生效。
	SubDir string

	// ── 语言配置 ──

	// Languages 显式指定语言列表（文件名前缀）。
	// 空 = 自动扫描目录下所有 *.json 并按前缀归并。
	// 非空 = 仅加载匹配前缀的文件，可用于屏蔽不需要的语言。
	Languages []string

	// DefaultLang 默认语言代码，如 "zh"。
	// 用于 FallbackDefaultLang 回退策略和 NormalizeLang 的 fallback。
	// 默认为 "en"。
	DefaultLang string

	// ── 校验配置 ──

	// StrictMode 控制 key 校验行为：
	//   true  — 任何语言缺少 key 时，Load 返回 error，阻止启动
	//   false — 缺少 key 时静默跳过，不阻塞启动（适合开发阶段逐步补充翻译）
	// 默认 false。
	StrictMode bool
}

// defaultConfig 返回 Config 的默认值。
func (c *Config) defaultConfig() {
	if c.DefaultLang == "" {
		c.DefaultLang = "en"
	}
}
