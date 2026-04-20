# 前端调用后端（Wails）

- **`APIInfo`**（`wails.ts`）：推荐在业务代码中使用的唯一入口，已包含账号、设置、补丁、刷新额度以及 **MITM** 相关方法。
- **`getAppStoragePath`**：号池 `accounts.json` / `settings.json` 所在目录（后端 `paths` 包解析，跨平台为 `UserConfigDir/WindsurfTools`，旧版 `windsurf-tools-wails` 会自动迁移）。
- **`wailsjs/go/main/App`**：Wails 生成的原始绑定；`APIInfo` 即为其薄封装，行为一致。
- 若新增 `app.go`（或 `main` 包）里导出给前端的函数，运行 `wails generate module`（或项目约定的生成命令）后，将对应项补到 `APIInfo` 中，避免部分页面直连 `App`、部分走 `APIInfo` 的分裂。
