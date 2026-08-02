// Package logit 提供结构化文本日志。
package logit

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultPodName 是 APP_NAME 环境变量未设置时的回退应用名称。
	DefaultPodName = "AppName"
)

// TimeCache 在日志热路径上缓存时间戳前缀以减少内存分配。
// 每个墙钟秒缓存 "YYYY-MM-DD HH:MM:SS" 部分。
// 当同一秒内发出多条日志时，可避免重复格式化前缀，只需每次追加毫秒部分。
//
// 线程安全：所有变更受互斥锁保护，防止并发访问缓存缓冲区。
type TimeCache struct {
	lastUnix int64      // 缓存值对应的 Unix 秒时间戳
	cache    []byte     // 缓存的 "YYYY-MM-DD HH:MM:SS" 字节
	mu       sync.Mutex // 保护并发访问的互斥锁
}

// appendFormattedTimestamp 将带毫秒的完整时间戳追加到缓冲区。
// 格式为 "2006-01-02 15:04:05.mmm"，其中 mmm 为 3 位毫秒。
// 若与前次调用处于同一秒内，则使用缓存前缀。可安全用于多 goroutine 并发。
func (c *TimeCache) appendFormattedTimestamp(buf *bytes.Buffer) {
	now := time.Now()
	unix := now.Unix()
	milli := int(now.UnixMilli() % 1000)

	c.mu.Lock()
	defer c.mu.Unlock()
	if unix != c.lastUnix {
		c.lastUnix = unix
		c.cache = now.AppendFormat(c.cache[:0], "2006-01-02 15:04:05")
	}
	buf.Write(c.cache)
	_, _ = fmt.Fprintf(buf, ".%03d", milli)
}

// timeCache 是用于时间戳缓存的全局单例。
var timeCache = &TimeCache{}

// callerCache 是将完整文件路径映射到短路径的并发缓存。
// 缩短后的输出仅保留 "dir/file.go" 而非完整路径，以减小日志体积。
// 使用 sync.Map 在并发日志场景下获得良好的读性能。
var callerCache sync.Map

// callerResolveCache 将 (格式 + 跳过条目指纹 + 栈 PC 前缀) 映射到解析后的调用者字符串。
// 相同的调用链 + 相同的合并策略 + 相同的格式命中缓存，避免在热路径上重复遍历帧。
var callerResolveCache sync.Map

// callerSkipPrefixesMatch 判断完全限定符号 fn 是否应被跳过。
// 每个条目携带自己的 Mode：
//   - CallerSkipPrefix：边界安全前缀匹配；"a/b" 匹配 "a/b.Foo" 和 "a/b/sub.Pkg"，但不匹配 "a/bextra.Foo"。
//   - CallerSkipContains：子串匹配；"logit" 匹配任何函数名中包含 "logit" 的帧。
func callerSkipPrefixesMatch(fn string, items []CallerSkipConfigItem) bool {
	for _, item := range items {
		if item.Value == "" {
			continue
		}
		switch item.Mode {
		case CallerSkipPrefix:
			if fn == item.Value || strings.HasPrefix(fn, item.Value+".") || strings.HasPrefix(fn, item.Value+"/") {
				return true
			}
		case CallerSkipContains:
			if strings.Contains(fn, item.Value) {
				return true
			}
		}
	}
	return false
}

// fingerprintCallerPrefixes 对合并后的跳过条目列表进行哈希，用于缓存键。
func fingerprintCallerPrefixes(items []CallerSkipConfigItem) uint64 {
	h := fnv.New64a()
	for _, item := range items {
		_, _ = h.Write([]byte{byte(item.Mode)})
		_, _ = h.Write([]byte(item.Value))
		_, _ = h.Write([]byte{0})
	}
	return h.Sum64()
}

// callerStackCacheKey 基于格式、前缀指纹及 runtime.Callers 返回的前几个 PC 构建稳定键。
func callerStackCacheKey(prefixFP uint64, pcs []uintptr, format CallerFormat) string {
	const maxPC = 16
	n := len(pcs)
	if n > maxPC {
		n = maxPC
	}
	var b strings.Builder
	b.Grow(24 + n*12)
	b.WriteByte(byte('0' + format))
	b.WriteByte('|')
	b.WriteString(strconv.FormatUint(prefixFP, 36))
	b.WriteByte('|')
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatUint(uint64(pcs[i]), 36))
	}
	return b.String()
}

// appendCallerFormatted 根据 format 参数将调用者位置追加到 buf：
//
//	CallerShort → "dir/file.go:行号"
//	CallerFull  → "/完整路径/dir/file.go:行号"
//	CallerFunc  → "import/path.Func:行号"
func appendCallerFormatted(buf *bytes.Buffer, file string, line int, fnName string, format CallerFormat) {
	switch format {
	case CallerFull:
		buf.WriteString(file)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(line))
	case CallerFunc:
		buf.WriteString(fnName)
		buf.WriteByte(':')
		buf.WriteString(strconv.Itoa(line))
	default:
		// CallerShort：取最后两级路径，使用 callerCache 缓存。
		appendCallerShortFile(buf, file, line)
	}
}

// appendCallerShortFile 为已知文件路径追加缩短后的 "dir/file.go:行号"。
func appendCallerShortFile(buf *bytes.Buffer, file string, line int) {
	shortFile, ok := callerCache.Load(file)
	if !ok {
		// 保留最后两级路径成分："package/file.go"
		idx := strings.LastIndexByte(file, '/')
		if idx != -1 {
			secondIdx := strings.LastIndexByte(file[:idx], '/')
			if secondIdx != -1 {
				shortFile = file[secondIdx+1:]
			} else {
				shortFile = file[idx+1:]
			}
		} else {
			shortFile = file
		}
		callerCache.Store(file, shortFile)
	}

	buf.WriteString(shortFile.(string))
	buf.WriteByte(':')
	buf.WriteString(strconv.Itoa(line))
}

// appendCaller 将调用者源码位置追加到缓冲区。
//
// skip 参数为跳过的栈帧数，语义与 runtime.Caller(skip) 一致（0 表示 appendCaller 内部的 runtime.Caller 自身）。
// skipItems 为调用者跳过规则列表，每个条目携带自己的 Mode（nil 表示不跳过，仅测试用）。
// prefixFP 在 skipItems 非 nil 时必须等于 fingerprintCallerPrefixes(skipItems)。
// format 控制渲染后的调用者字符串（CallerShort / CallerFull / CallerFunc）。
func appendCaller(buf *bytes.Buffer, skip int, skipItems []CallerSkipConfigItem, prefixFP uint64, format CallerFormat) {
	const maxFrames = 64
	pcs := make([]uintptr, maxFrames)
	n := runtime.Callers(skip+1, pcs)
	pcs = pcs[:n]

	cacheKey := callerStackCacheKey(prefixFP, pcs, format)
	if v, ok := callerResolveCache.Load(cacheKey); ok {
		buf.WriteString(v.(string))
		return
	}

	frames := runtime.CallersFrames(pcs)
	for {
		frame, more := frames.Next()
		if frame.Function == "" {
			if !more {
				break
			}
			continue
		}
		if len(skipItems) > 0 && callerSkipPrefixesMatch(frame.Function, skipItems) {
			if !more {
				break
			}
			continue
		}
		var tmp bytes.Buffer
		appendCallerFormatted(&tmp, frame.File, frame.Line, frame.Function, format)
		resolved := tmp.String()
		callerResolveCache.Store(cacheKey, resolved)
		buf.WriteString(resolved)
		return
	}
	const unknown = "unknown:0"
	callerResolveCache.Store(cacheKey, unknown)
	buf.WriteString(unknown)
}

// appNameCache 缓存从 APP_NAME 环境变量读取的应用名称。
var appNameCache []byte

// getAppName 返回从 APP_NAME 环境变量读取的缓存应用名称。
// 若 APP_NAME 未设置或为空，回退到 DefaultPodName。首次调用后结果被缓存。
func getAppName() []byte {
	if len(appNameCache) > 0 {
		return appNameCache
	}
	appName := os.Getenv("APP_NAME")
	if appName == "" {
		appName = DefaultPodName
	}
	appNameCache = []byte(appName)
	return appNameCache
}
