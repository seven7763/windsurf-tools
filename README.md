# Windsurf Tools

基于 [Wails v2](https://wails.io/)（Go + Vue 3）的桌面应用，当前前端产品形态已经收口为 **纯 MITM 工作流**，用于管理 **Windsurf MITM 号池、多账号额度同步、凭证刷新、MITM 无感换号** 与 **OpenAI Relay**。

**GitHub 仓库：** [shaoyu521/windsurf-Tools](https://github.com/shaoyu521/windsurf-Tools)

---

## 下载发布包

每次推送 `v*` 标签后，GitHub Actions 会自动构建并发布以下产物到 [Releases](https://github.com/shaoyu521/windsurf-Tools/releases)：

| 文件 | 平台 | 说明 |
|------|------|------|
| `windsurf-tools-wails.exe` | Windows amd64 | 单文件，启动时默认请求管理员权限 |
| `windsurf-tools-wails-windows-amd64.zip` | Windows amd64 | Windows 单文件压缩包 |
| `windsurf-tools-wails-macos-intel-amd64.zip` | macOS Intel | 打包后的 `.app` 压缩包 |
| `windsurf-tools-wails-macos-apple-silicon-arm64.zip` | macOS Apple Silicon | 打包后的 `.app` 压缩包 |
| `SHA256SUMS.txt` | 全平台 | 所有发布文件的 SHA256 校验 |

> macOS 发布包当前为 **未签名** 构建，首次打开可能需要在系统设置中允许应用运行。
>
> 为了保证 Intel / Apple Silicon 双架构发布稳定，当前 macOS 包默认关闭系统托盘集成；账号池、MITM、额度同步、桌面工具栏与单实例逻辑均可正常使用。
>
> Windows 包当前默认以管理员权限启动，便于直接完成 Hosts、CA 证书、系统服务与代理相关操作。

---

## 界面预览

| 最新主界面预览 |
| :---: |
| ![Windsurf Tools 最新界面预览](docs/images/dashboard-preview.png) |

> 当前仓库预览图已同步为最新桌面端截图，展示控制台首页、MITM 面板与健康度概览。源文件位于 `docs/images/dashboard-preview.png` 与 `build/browser-preview.png`。

---

## 主要功能

- **账号池管理**：支持批量导入邮箱密码、Refresh Token、API Key、JWT；支持搜索、计划分组、快速删除免费账号和清理过期账号。
- **纯 MITM 无感换号**：通过 Hosts 劫持 + 本地 TLS MITM + JWT 替换 + API Key 轮换实现主路径无感切号，当前前端默认不再暴露本地 auth 文件切号流程。
- **OpenAI Relay**：本地提供 OpenAI 兼容 API 入口，直接复用 MITM 号池和轮换逻辑。
- **统一到期时间与额度展示**：账号卡片聚焦账号、邮箱、到期时间、额度状态，围绕纯 MITM 号池的可用性展示当前活跃状态与运行时见底信息。
- **托盘与桌面工具栏**：支持关闭隐藏到托盘、静默启动、桌面小工具栏模式。
- **单实例运行**：重复打开应用时不再堆多个窗口，而是激活已有实例。
- **退出自动恢复环境**：当未开启“关闭时隐藏至系统托盘”时，点击关闭会真正退出，并自动恢复 MITM 涉及的 `hosts / ProxyOverride / Codeium 配置 / CA`。
- **服务停止自动清理**：系统服务停止时也会执行同样的 MITM 环境恢复，避免端口、Hosts 和证书残留。

---

## 运行环境

### Windows

- Windows 10 / 11
- `amd64`
- 需要 [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
- 启动时默认请求管理员权限，用于管理 Hosts、证书、服务与代理环境

### macOS

- 支持 **Intel (`amd64`)** 与 **Apple Silicon (`arm64`)**
- 当前通过 GitHub Actions 分别在 `macos-15-intel`（Intel）与 `macos-15`（Apple Silicon）原生 runner 上构建发布包
- 当前为未签名构建，首次打开可能需要绕过 Gatekeeper
- 当前 macOS 发布包默认不启用系统托盘，因此“关闭时隐藏至系统托盘”不会生效；点击关闭会真正退出并恢复 MITM 环境

---

## 从源码构建

### 前置条件

- [Go](https://go.dev/dl/) 1.24.x
- [Node.js](https://nodejs.org/) 20+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

```bash
wails doctor
```

### 构建步骤

```bash
git clone https://github.com/shaoyu521/windsurf-Tools.git
cd windsurf-Tools

cd frontend
npm install
cd ..

wails build
```

默认输出：

- Windows: `build/bin/windsurf-tools-wails.exe`
- macOS: `build/bin/windsurf-tools-wails.app`

如需在对应平台显式指定架构：

```bash
wails build -platform windows/amd64
wails build -platform darwin/amd64
wails build -platform darwin/arm64
```

开发调试：

```bash
wails dev
```

---

## 托盘、关闭与静默启动

- `windsurf-tools-wails.exe --silent`：启动后不弹主窗口，仍可从托盘唤醒。
- 开启“关闭时隐藏至系统托盘”：点击右上角关闭只隐藏窗口，不退出进程。
- 关闭“关闭时隐藏至系统托盘”：点击右上角关闭会真正退出，并自动恢复 MITM 环境。
- 托盘菜单“退出并恢复环境”：无论当前是否显示主窗口，都会完整退出并触发环境清理。
- macOS Intel / Apple Silicon 发布包为兼容构建，默认不启用系统托盘；若开启桌面工具栏，可继续用静默启动直接进入小工具栏模式。

---

## 系统服务（无界面后台）

可执行文件支持 [kardianos/service](https://github.com/kardianos/service) 子命令：

| 子命令 | 说明 |
|--------|------|
| `install` | 注册系统服务 |
| `uninstall` | 卸载服务 |
| `start` / `stop` / `restart` | 启停服务 |

服务进程无 WebView、无托盘，只跑后台逻辑与可选 MITM。若 `settings.json` 中开启了 `mitm_proxy_enabled`，服务启动时会自动尝试拉起 MITM。

服务停止时会：

- 停止 MITM 监听
- 恢复 `hosts`
- 清理 `ProxyOverride`
- 恢复 `.codeium/config.json`
- 卸载 MITM CA

> 若服务以 `LocalSystem` 或 `root` 运行，配置目录与当前登录用户的桌面版通常不是同一路径。

---

## 数据目录

默认数据目录与代码中的 `backend/paths` / `backend/store` 一致，统一位于用户配置目录下的 `WindsurfTools`：

- Windows 典型路径：`%APPDATA%\\WindsurfTools\\accounts.json`
- Windows 典型路径：`%APPDATA%\\WindsurfTools\\settings.json`

旧版 `windsurf-tools-wails` 目录会在首次启动时自动迁移。

---

## 数据与隐私

- 请勿将包含真实账号、密码、Refresh Token、JWT、API Key 的本地数据文件提交到 Git 仓库。
- 请勿公开分享本机生成的 `accounts.json` / `settings.json`。
- 完整安全约定见 [SECURITY.md](SECURITY.md)。

---

## 辅助脚本

仓库 `tools/` 目录保留若干 Node / Python 辅助脚本，用于批处理导入、格式转换等。运行前请按脚本说明设置对应环境变量，并避免把真实凭证直接写死到脚本文件中。

---

## 项目结构

```text
.
├── app_*.go              # Wails 绑定拆分后的业务入口
├── main.go               # 程序入口与单实例 / 窗口配置
├── service.go            # 系统服务入口
├── tray.go               # 系统托盘
├── backend/              # 模型、路径、存储、MITM/刷新服务
├── frontend/             # Vue 3 + Vite + Tailwind
├── build/                # 图标、plist、manifest、预览图
├── docs/images/          # README 截图
└── wails.json
```

---

## 维护者发布说明

仓库已配置 [GitHub Actions 发布工作流](.github/workflows/release-windows.yml)。推送 `v*` 标签后会自动：

1. 在 `windows-latest` 上构建 Windows `amd64`
2. 在 `macos-15-intel` 上构建 macOS Intel `amd64`
3. 在 `macos-15` 上构建 macOS Apple Silicon `arm64`
4. 汇总生成 `SHA256SUMS.txt`
5. 自动发布到 GitHub Release

示例：

```bash
git tag v0.2.1 -m "v0.2.1"
git push origin main
git push origin v0.2.1
```

请确认仓库已启用 GitHub Actions，且 `GITHUB_TOKEN` 拥有 `contents: write` 权限。

---

## 开源许可

[MIT License](LICENSE)
