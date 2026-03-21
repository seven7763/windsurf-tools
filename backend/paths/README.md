# 应用数据路径

| 用途 | 目录 |
|------|------|
| 号池 `accounts.json`、`settings.json` | `ResolveAppConfigDir()` → `%APPDATA%\WindsurfTools` / `~/Library/Application Support/WindsurfTools` / `$XDG_CONFIG_HOME/WindsurfTools` |
| 旧版（自动迁移） | `windsurf-tools-wails` |
| MITM 根证书（与号池分离） | `~/.windsurf-tools/`（见 `services/proxy_cert.go`） |

迁移：若仅旧目录存在数据，会复制到 `WindsurfTools` 并在旧目录写入 `.migrated_to_WindsurfTools` 标记。
