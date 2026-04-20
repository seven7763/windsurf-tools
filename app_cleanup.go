package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ═══════════════════════════════════════
// Windsurf 清理 & 性能优化
// ═══════════════════════════════════════

// CleanupCategory 清理类别
type CleanupCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeHuman   string `json:"size_human"`
	FileCount   int    `json:"file_count"`
	Safe        bool   `json:"safe"` // true=不影响对话历史
}

// CleanupResult 清理结果
type CleanupResult struct {
	Category    string `json:"category"`
	Success     bool   `json:"success"`
	FreedBytes  int64  `json:"freed_bytes"`
	FreedHuman  string `json:"freed_human"`
	DeletedDirs int    `json:"deleted_dirs"`
	Error       string `json:"error,omitempty"`
}

// WindsurfDiskUsage 磁盘占用分析
type WindsurfDiskUsage struct {
	Categories []CleanupCategory `json:"categories"`
	TotalBytes int64             `json:"total_bytes"`
	TotalHuman string            `json:"total_human"`
}

// PerformanceTip 性能优化建议
type PerformanceTip struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"` // high / medium / low
	AutoFix     bool   `json:"auto_fix"`
}

// ── Windows 路径 ──

func windsurfCodeiumDir() string {
	return filepath.Join(os.Getenv("USERPROFILE"), ".codeium", "windsurf")
}

func windsurfAppDataDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "Windsurf")
}

// cleanupPaths 返回所有可清理路径
func cleanupPaths() map[string][]string {
	codeiumDir := windsurfCodeiumDir()
	appDataDir := windsurfAppDataDir()
	return map[string][]string{
		"cascade": {
			filepath.Join(codeiumDir, "cascade"),
		},
		"gpu_cache": {
			filepath.Join(appDataDir, "GPUCache"),
		},
		"code_cache": {
			filepath.Join(appDataDir, "Code Cache"),
		},
		"cached_data": {
			filepath.Join(codeiumDir, "CachedData"),
		},
		"cached_extensions": {
			filepath.Join(codeiumDir, "CachedExtensions"),
		},
		"logs": {
			filepath.Join(appDataDir, "logs"),
		},
		"workspace_storage": {
			filepath.Join(appDataDir, "User", "workspaceStorage"),
		},
		"crash_reports": {
			filepath.Join(appDataDir, "Crashpad"),
			filepath.Join(codeiumDir, "CachedProfileData"),
		},
		"indexeddb": {
			filepath.Join(appDataDir, "IndexedDB"),
		},
	}
}

// ── 磁盘分析 ──

// GetWindsurfDiskUsage 分析 Windsurf 各缓存目录的磁盘占用
func (a *App) GetWindsurfDiskUsage() WindsurfDiskUsage {
	categories := []struct {
		id          string
		name        string
		description string
		safe        bool
	}{
		{"cascade", "Cascade 对话缓存", "AI 对话历史和本地设置（清理后需重新建立对话）", false},
		{"gpu_cache", "GPU 渲染缓存", "GPU 加速渲染缓存，清理后自动重建", true},
		{"code_cache", "代码缓存", "V8 编译代码缓存，清理后自动重建", true},
		{"cached_data", "编辑器缓存数据", "编辑器内部缓存数据", true},
		{"cached_extensions", "扩展缓存", "已安装扩展的缓存副本", true},
		{"logs", "日志文件", "运行日志，清理 7 天以前的旧日志", true},
		{"workspace_storage", "工作区存储", "各项目工作区的本地状态（可能很大）", true},
		{"crash_reports", "崩溃报告 & 缓存配置", "Crashpad 崩溃报告和缓存的配置数据", true},
		{"indexeddb", "IndexedDB 数据", "本地 IndexedDB 存储数据", true},
	}

	paths := cleanupPaths()
	var result WindsurfDiskUsage

	for _, cat := range categories {
		cc := CleanupCategory{
			ID:          cat.id,
			Name:        cat.name,
			Description: cat.description,
			Safe:        cat.safe,
		}
		dirs := paths[cat.id]
		for _, dir := range dirs {
			size, count := dirSizeAndCount(dir)
			cc.SizeBytes += size
			cc.FileCount += count
		}
		cc.SizeHuman = humanSize(cc.SizeBytes)
		result.TotalBytes += cc.SizeBytes
		result.Categories = append(result.Categories, cc)
	}
	result.TotalHuman = humanSize(result.TotalBytes)
	return result
}

// CleanupWindsurf 清理指定类别
func (a *App) CleanupWindsurf(categoryIDs []string) []CleanupResult {
	paths := cleanupPaths()
	var results []CleanupResult

	for _, id := range categoryIDs {
		dirs, ok := paths[id]
		if !ok {
			results = append(results, CleanupResult{
				Category: id,
				Success:  false,
				Error:    "未知的清理类别",
			})
			continue
		}

		var totalFreed int64
		var totalDeleted int
		var errs []string

		for _, dir := range dirs {
			if id == "logs" {
				// 日志只清理 7 天以前的文件
				freed, deleted, err := cleanOldFiles(dir, 7*24*time.Hour)
				totalFreed += freed
				totalDeleted += deleted
				if err != nil {
					errs = append(errs, err.Error())
				}
			} else {
				size, _ := dirSizeAndCount(dir)
				if err := os.RemoveAll(dir); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", dir, err))
				} else {
					totalFreed += size
					totalDeleted++
				}
			}
		}

		cr := CleanupResult{
			Category:    id,
			Success:     len(errs) == 0,
			FreedBytes:  totalFreed,
			FreedHuman:  humanSize(totalFreed),
			DeletedDirs: totalDeleted,
		}
		if len(errs) > 0 {
			cr.Error = strings.Join(errs, "; ")
			cr.Success = totalFreed > 0 // 部分成功
		}
		results = append(results, cr)
	}
	return results
}

// CleanupStartupCache 一键清理启动缓存（不影响对话历史）
func (a *App) CleanupStartupCache() []CleanupResult {
	return a.CleanupWindsurf([]string{
		"gpu_cache", "code_cache", "cached_data", "cached_extensions", "logs",
	})
}

// CleanupAllSafe 清理所有安全类别（不含 Cascade 对话）
func (a *App) CleanupAllSafe() []CleanupResult {
	return a.CleanupWindsurf([]string{
		"gpu_cache", "code_cache", "cached_data", "cached_extensions",
		"logs", "workspace_storage", "crash_reports", "indexeddb",
	})
}

// ── 性能优化建议 ──

// GetPerformanceTips 获取 Windsurf 性能优化建议
func (a *App) GetPerformanceTips() []PerformanceTip {
	tips := []PerformanceTip{
		{
			ID:          "disable_telemetry",
			Title:       "关闭遥测数据收集",
			Description: "在 settings.json 中设置 \"telemetry.telemetryLevel\": \"off\"，减少后台网络请求和数据处理",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_minimap",
			Title:       "关闭代码缩略图",
			Description: "设置 \"editor.minimap.enabled\": false，减少渲染开销",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "reduce_file_watchers",
			Title:       "排除大目录的文件监控",
			Description: "在 files.watcherExclude 中添加 node_modules、.git、dist 等大目录，大幅降低 CPU 占用",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_search_follow_symlinks",
			Title:       "搜索时不跟随符号链接",
			Description: "设置 \"search.followSymlinks\": false，避免搜索时遍历符号链接导致的重复扫描",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "limit_editor_tokens",
			Title:       "限制大文件的语法高亮",
			Description: "设置 \"editor.maxTokenizationLineLength\": 5000，避免超长行导致 CPU 飙升",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_gpu_accel",
			Title:       "禁用 GPU 硬件加速",
			Description: "当 GPU 驱动不稳定时，关闭 GPU 加速可降低内存占用和避免渲染卡顿。启动参数添加 --disable-gpu",
			Impact:      "high",
			AutoFix:     false,
		},
		{
			ID:          "reduce_terminal_scrollback",
			Title:       "减少终端滚动缓冲区",
			Description: "设置 \"terminal.integrated.scrollback\": 1000（默认 1000 行，有些用户设很大值），减少内存占用",
			Impact:      "low",
			AutoFix:     true,
		},
		{
			ID:          "limit_open_editors",
			Title:       "限制打开的编辑器数量",
			Description: "设置 \"workbench.editor.limit.enabled\": true 和 \"workbench.editor.limit.value\": 8，减少打开的标签页占用内存",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "exclude_large_folders",
			Title:       "排除大文件夹的文件搜索",
			Description: "在 files.exclude 中添加 build、dist、.cache 等输出目录",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_bracket_pair",
			Title:       "关闭括号对指引线",
			Description: "设置 \"editor.guides.bracketPairs\": false，减少渲染计算",
			Impact:      "low",
			AutoFix:     true,
		},
		// ── 以下为深度降内存项 ──
		{
			ID:          "limit_ts_memory",
			Title:       "限制 TypeScript 服务器内存",
			Description: "设置 \"typescript.tsserver.maxTsServerMemory\": 2048，防止 TS 语言服务占用过多内存（默认无限制可达 4GB+）",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_git_decorations",
			Title:       "关闭 Git 装饰与自动抓取",
			Description: "关闭 git.decorations / git.autofetch / git.autoRepositoryDetection，大幅减少 Git 进程的 CPU 和内存消耗",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_semantic_highlight",
			Title:       "关闭语义高亮",
			Description: "设置 \"editor.semanticHighlighting.enabled\": false，语义高亮需要语言服务器额外分析，关闭后可显著降低内存",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_sticky_scroll",
			Title:       "关闭粘性滚动",
			Description: "设置 \"editor.stickyScroll.enabled\": false，减少编辑器渲染开销",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_word_suggestions",
			Title:       "关闭基于单词的建议",
			Description: "设置 \"editor.wordBasedSuggestions\": \"off\"，减少自动补全内存缓存",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "enable_large_file_opt",
			Title:       "启用大文件优化模式",
			Description: "设置 \"editor.largeFileOptimizations\": true，对大文件自动禁用高开销功能",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_breadcrumbs",
			Title:       "关闭面包屑导航",
			Description: "设置 \"breadcrumbs.enabled\": false，减少符号解析的内存占用",
			Impact:      "low",
			AutoFix:     true,
		},
		{
			ID:          "reduce_diff_max_size",
			Title:       "缩小 Diff 编辑器文件上限",
			Description: "设置 \"diffEditor.maxFileSize\": 5，限制 diff 编辑器最大文件为 5MB，避免大文件 diff 吃掉数 GB 内存",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "disable_auto_imports",
			Title:       "关闭 TypeScript 自动导入建议",
			Description: "设置 \"typescript.suggest.autoImports\": false 和 \"javascript.suggest.autoImports\": false，减少 TS 服务器扫描全项目的内存消耗",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_inline_suggest",
			Title:       "关闭内联建议（Ghost Text）",
			Description: "设置 \"editor.inlineSuggest.enabled\": false，关闭编辑器内联 AI 建议的幽灵文本渲染",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "limit_v8_heap",
			Title:       "限制 V8 堆内存上限 (4GB)",
			Description: "在 argv.json 中设置 --max-old-space-size=4096，限制每个 Electron 进程的 V8 堆不超过 4GB。需重启 Windsurf 生效",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_extensions_auto_update",
			Title:       "关闭扩展自动更新",
			Description: "设置 \"extensions.autoUpdate\": false 和 \"extensions.autoCheckUpdates\": false，减少后台网络和 CPU 占用",
			Impact:      "medium",
			AutoFix:     true,
		},
		{
			ID:          "reduce_editor_hover_delay",
			Title:       "减少悬停预计算",
			Description: "设置 \"editor.hover.delay\": 600 和 \"editor.parameterHints.enabled\": false，减少语言服务器的实时分析负担",
			Impact:      "medium",
			AutoFix:     true,
		},
		// ── 以下为 Codeium Language Server 降内存项（每个窗口 1~3GB 的真凶）──
		{
			ID:          "limit_codeium_indexing",
			Title:       "★ 限制 AI 索引文件数量",
			Description: "设置 \"windsurf.indexing.maxFileCount\": 1000，Codeium Language Server 默认索引 5000 个文件，每个窗口占 1~3GB 内存。降到 1000 可大幅缓解",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "disable_supercomplete",
			Title:       "★ 关闭 Supercomplete（多行预测）",
			Description: "设置 \"windsurf.supercomplete.enabled\": false，Supercomplete 会预计算多行补全建议，占用大量 Language Server 内存",
			Impact:      "high",
			AutoFix:     true,
		},
		{
			ID:          "reduce_autocomplete_speed",
			Title:       "降低自动补全速度为 balanced",
			Description: "设置 \"windsurf.autocompleteSpeed\": \"balanced\"，从 fast 切换为 balanced 减少预计算和缓存内存",
			Impact:      "medium",
			AutoFix:     true,
		},
	}
	return tips
}

// ApplyPerformanceFix 应用指定的性能优化
func (a *App) ApplyPerformanceFix(tipIDs []string) map[string]string {
	settingsPath := filepath.Join(windsurfAppDataDir(), "User", "settings.json")
	results := make(map[string]string)

	// 读取现有 settings
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		for _, id := range tipIDs {
			results[id] = "读取 settings.json 失败: " + err.Error()
		}
		return results
	}

	text := string(content)
	modified := false

	for _, id := range tipIDs {
		switch id {
		case "disable_telemetry":
			if applied := injectSettingIfMissing(&text, `"telemetry.telemetryLevel"`, `"telemetry.telemetryLevel": "off"`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_minimap":
			if applied := injectSettingIfMissing(&text, `"editor.minimap.enabled"`, `"editor.minimap.enabled": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "reduce_file_watchers":
			if applied := injectWatcherExcludes(&text); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_search_follow_symlinks":
			if applied := injectSettingIfMissing(&text, `"search.followSymlinks"`, `"search.followSymlinks": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "limit_editor_tokens":
			if applied := injectSettingIfMissing(&text, `"editor.maxTokenizationLineLength"`, `"editor.maxTokenizationLineLength": 5000`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "reduce_terminal_scrollback":
			if applied := injectSettingIfMissing(&text, `"terminal.integrated.scrollback"`, `"terminal.integrated.scrollback": 1000`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "limit_open_editors":
			a1 := injectSettingIfMissing(&text, `"workbench.editor.limit.enabled"`, `"workbench.editor.limit.enabled": true`)
			a2 := injectSettingIfMissing(&text, `"workbench.editor.limit.value"`, `"workbench.editor.limit.value": 8`)
			if a1 || a2 {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "exclude_large_folders":
			if applied := injectFilesExcludes(&text); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_bracket_pair":
			if applied := injectSettingIfMissing(&text, `"editor.guides.bracketPairs"`, `"editor.guides.bracketPairs": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_gpu_accel":
			results[id] = "需手动操作: 在 Windsurf 快捷方式中添加 --disable-gpu 启动参数"
		// ── 深度降内存项 ──
		case "limit_ts_memory":
			if applied := injectSettingIfMissing(&text, `"typescript.tsserver.maxTsServerMemory"`, `"typescript.tsserver.maxTsServerMemory": 2048`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_git_decorations":
			a1 := injectSettingIfMissing(&text, `"git.decorations.enabled"`, `"git.decorations.enabled": false`)
			a2 := injectSettingIfMissing(&text, `"git.autofetch"`, `"git.autofetch": false`)
			a3 := injectSettingIfMissing(&text, `"git.autoRepositoryDetection"`, `"git.autoRepositoryDetection": false`)
			a4 := injectSettingIfMissing(&text, `"git.enabled"`, `"git.enabled": false`)
			if a1 || a2 || a3 || a4 {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_semantic_highlight":
			if applied := injectSettingIfMissing(&text, `"editor.semanticHighlighting.enabled"`, `"editor.semanticHighlighting.enabled": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_sticky_scroll":
			if applied := injectSettingIfMissing(&text, `"editor.stickyScroll.enabled"`, `"editor.stickyScroll.enabled": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_word_suggestions":
			if applied := injectSettingIfMissing(&text, `"editor.wordBasedSuggestions"`, `"editor.wordBasedSuggestions": "off"`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "enable_large_file_opt":
			if applied := injectSettingIfMissing(&text, `"editor.largeFileOptimizations"`, `"editor.largeFileOptimizations": true`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_breadcrumbs":
			if applied := injectSettingIfMissing(&text, `"breadcrumbs.enabled"`, `"breadcrumbs.enabled": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "reduce_diff_max_size":
			if applied := injectSettingIfMissing(&text, `"diffEditor.maxFileSize"`, `"diffEditor.maxFileSize": 5`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_auto_imports":
			a1 := injectSettingIfMissing(&text, `"typescript.suggest.autoImports"`, `"typescript.suggest.autoImports": false`)
			a2 := injectSettingIfMissing(&text, `"javascript.suggest.autoImports"`, `"javascript.suggest.autoImports": false`)
			if a1 || a2 {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_inline_suggest":
			if applied := injectSettingIfMissing(&text, `"editor.inlineSuggest.enabled"`, `"editor.inlineSuggest.enabled": false`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "limit_v8_heap":
			// argv.json 在 %USERPROFILE%\.windsurf\argv.json 或 %APPDATA%\Windsurf\argv.json
			argvResult := applyV8HeapLimit()
			results[id] = argvResult
			// argv.json 修改不算 settings.json modified
		case "disable_extensions_auto_update":
			a1 := injectSettingIfMissing(&text, `"extensions.autoUpdate"`, `"extensions.autoUpdate": false`)
			a2 := injectSettingIfMissing(&text, `"extensions.autoCheckUpdates"`, `"extensions.autoCheckUpdates": false`)
			if a1 || a2 {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "reduce_editor_hover_delay":
			a1 := injectSettingIfMissing(&text, `"editor.hover.delay"`, `"editor.hover.delay": 600`)
			a2 := injectSettingIfMissing(&text, `"editor.parameterHints.enabled"`, `"editor.parameterHints.enabled": false`)
			if a1 || a2 {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		// ── Codeium Language Server 降内存项 ──
		case "limit_codeium_indexing":
			if applied := injectSettingIfMissing(&text, `"windsurf.indexing.maxFileCount"`, `"windsurf.indexing.maxFileCount": 1000`); applied {
				results[id] = "已应用（需重启 Windsurf 生效）"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "disable_supercomplete":
			if applied := injectSettingIfMissing(&text, `"windsurf.supercomplete.enabled"`, `"windsurf.supercomplete.enabled": false`); applied {
				results[id] = "已应用（需重启 Windsurf 生效）"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		case "reduce_autocomplete_speed":
			if applied := injectSettingIfMissing(&text, `"windsurf.autocompleteSpeed"`, `"windsurf.autocompleteSpeed": "balanced"`); applied {
				results[id] = "已应用"
				modified = true
			} else {
				results[id] = "已存在，跳过"
			}
		default:
			results[id] = "未知的优化项"
		}
	}

	if modified {
		// 备份
		backupPath := settingsPath + ".bak." + time.Now().Format("20060102_150405")
		_ = os.WriteFile(backupPath, content, 0644)
		if err := os.WriteFile(settingsPath, []byte(text), 0644); err != nil {
			for id, v := range results {
				if v == "已应用" {
					results[id] = "写入失败: " + err.Error()
				}
			}
		}
	}
	return results
}

// ApplyAllPerformanceFixes 一键应用所有可自动修复的性能优化
func (a *App) ApplyAllPerformanceFixes() map[string]string {
	tips := a.GetPerformanceTips()
	var ids []string
	for _, tip := range tips {
		if tip.AutoFix {
			ids = append(ids, tip.ID)
		}
	}
	return a.ApplyPerformanceFix(ids)
}

// GetWindsurfProcessInfo 获取 Windsurf 进程内存/CPU 信息
func (a *App) GetWindsurfProcessInfo() []map[string]interface{} {
	if runtime.GOOS != "windows" {
		return nil
	}
	// 使用 tasklist 获取 Windsurf 相关进程
	return getWindsurfProcesses()
}

// ── 辅助函数 ──

func dirSizeAndCount(path string) (int64, int) {
	var totalSize int64
	var count int
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			totalSize += info.Size()
			count++
		}
		return nil
	})
	return totalSize, count
}

func cleanOldFiles(dir string, maxAge time.Duration) (int64, int, error) {
	cutoff := time.Now().Add(-maxAge)
	var freed int64
	var deleted int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			size := info.Size()
			if removeErr := os.Remove(path); removeErr == nil {
				freed += size
				deleted++
			}
		}
		return nil
	})
	return freed, deleted, err
}

func humanSize(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024
	if kb < 1024 {
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024
	return fmt.Sprintf("%.2f GB", gb)
}

// injectSettingIfMissing 在 settings.json 的最后一个 } 前插入设置（如果不存在）
func injectSettingIfMissing(text *string, key, fullLine string) bool {
	if strings.Contains(*text, key) {
		return false
	}
	// 找到最后一个 }
	lastBrace := strings.LastIndex(*text, "}")
	if lastBrace < 0 {
		return false
	}
	// 检查前面是否需要逗号
	prefix := strings.TrimRight((*text)[:lastBrace], " \t\n\r")
	needComma := len(prefix) > 0 && prefix[len(prefix)-1] != '{' && prefix[len(prefix)-1] != ','
	inject := "\n    " + fullLine
	if needComma {
		inject = "," + inject
	}
	*text = (*text)[:lastBrace] + inject + "\n" + (*text)[lastBrace:]
	return true
}

func injectWatcherExcludes(text *string) bool {
	if strings.Contains(*text, `"files.watcherExclude"`) {
		return false
	}
	block := `"files.watcherExclude": {
        "**/node_modules/**": true,
        "**/.git/objects/**": true,
        "**/.git/subtree-cache/**": true,
        "**/dist/**": true,
        "**/build/**": true,
        "**/.cache/**": true,
        "**/vendor/**": true
    }`
	return injectSettingIfMissing(text, `"files.watcherExclude"`, block)
}

func injectFilesExcludes(text *string) bool {
	if strings.Contains(*text, `"files.exclude"`) {
		return false
	}
	block := `"files.exclude": {
        "**/node_modules": true,
        "**/.git": true,
        "**/dist": true,
        "**/build": true,
        "**/.cache": true,
        "**/__pycache__": true,
        "**/.DS_Store": true
    }`
	return injectSettingIfMissing(text, `"files.exclude"`, block)
}

// applyV8HeapLimit 在 Windsurf 的 argv.json 中添加 --max-old-space-size=4096
// argv.json 可能在 %USERPROFILE%\.windsurf\argv.json
func applyV8HeapLimit() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		home = os.Getenv("HOME")
	}
	candidates := []string{
		filepath.Join(home, ".windsurf", "argv.json"),
		filepath.Join(windsurfAppDataDir(), "argv.json"),
	}

	var argvPath string
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			argvPath = p
			break
		}
	}

	if argvPath == "" {
		// 创建默认 argv.json
		argvPath = candidates[0]
		dir := filepath.Dir(argvPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "创建目录失败: " + err.Error()
		}
		content := "// This configuration file allows you to pass permanent command line arguments to Windsurf.\n" +
			"// Only a subset of arguments are currently supported.\n" +
			"{\n    \"js-flags\": \"--max-old-space-size=4096\"\n}\n"
		if err := os.WriteFile(argvPath, []byte(content), 0644); err != nil {
			return "写入 argv.json 失败: " + err.Error()
		}
		return "已应用 (新建 " + argvPath + ")"
	}

	// 读取已有
	data, err := os.ReadFile(argvPath)
	if err != nil {
		return "读取 argv.json 失败: " + err.Error()
	}
	text := string(data)

	if strings.Contains(text, "max-old-space-size") {
		return "已存在，跳过"
	}

	// 备份
	backupPath := argvPath + ".bak." + time.Now().Format("20060102_150405")
	_ = os.WriteFile(backupPath, data, 0644)

	// 注入 js-flags
	if strings.Contains(text, `"js-flags"`) {
		// 已有 js-flags，追加参数
		// 简单替换: 在 js-flags 的值末尾追加
		text = strings.Replace(text, `"js-flags": "`, `"js-flags": "--max-old-space-size=4096 `, 1)
	} else {
		// 没有 js-flags，注入
		if applied := injectSettingIfMissing(&text, `"js-flags"`, `"js-flags": "--max-old-space-size=4096"`); !applied {
			return "注入 js-flags 失败"
		}
	}

	if err := os.WriteFile(argvPath, []byte(text), 0644); err != nil {
		return "写入 argv.json 失败: " + err.Error()
	}
	return "已应用 (需重启 Windsurf 生效)"
}
