package main

// GetAppStoragePath 返回号池（accounts.json）与 settings.json 所在目录，跨平台为：
// Windows: %APPDATA%\WindsurfTools；macOS: ~/Library/Application Support/WindsurfTools；Linux: ~/.config/WindsurfTools。
// 若自旧版 windsurf-tools-wails 迁移过，目录已为新版路径。
func (a *App) GetAppStoragePath() string {
	if a.store == nil {
		return ""
	}
	return a.store.DataDir()
}
