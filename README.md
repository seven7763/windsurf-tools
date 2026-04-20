# Windsurf Tools 🏄‍♂️

[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS-blue)](#运行环境--prerequisites)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Built with Wails](https://img.shields.io/badge/Built%20with-Wails%20v2-red)](https://wails.io/)

> **Windsurf IDE 号池 + 纯 MITM 代理一体化工具**
> Seamless MITM proxy for Windsurf IDE — account pool rotation, billing identity rewrite, quota sync, and a local OpenAI-compatible relay.

基于 [Wails v2](https://wails.io/) (Go + Vue 3) 的桌面工具，为 Windsurf / Codeium IDE 提供：

- 🕵️ **纯 MITM 代理** — 劫持 `server.codeium.com` / `server.self-serve.windsurf.com`，在 protobuf 层替换 `sk-ws-` key、JWT、**F20 UserID / F32 TeamID 计费字段**，让上游按号池账号扣费而不是登录账号
- 🎯 **号池动态切换** — Free / Trial / Pro / Max 多套餐统一管理，按会话粘性分配 pool key，避免 Cascade session 失效
- 📊 **实时用量 & 诊断** — 统计 Windsurf / OpenAI 方向 token 流水，聚合美金成本，带完整请求审计
- � **本地 OpenAI Relay** — SSE 流式输出，兼容 `OpenAI SDK` / `LobeChat` / `ChatGPT-Next-Web` / `Cursor`，自带健康检测和故障倒换
- �️ **清道夫** — 一键清理 Cascade 对话残留和渲染缓存
- 🔐 **单密码特权操作** — macOS 合并 CA 信任 / hosts 写入 / 端口 443 绑定为一次 osascript 弹窗

---

## 🎨 界面缩略与核心功能 | Features & Previews

#### 1. 代理核心与全局总览 (Dashboard)
直观的全局大盘！一眼确认纯 MITM 代理状态、健康度、号池总量与活跃的无感切割链路，以及中转大盘信息。

| 首页总览面板 |
| :---: |
| ![Dashboard](docs/images/preview-dashboard.png) |

#### 2. 号池统管全景 (Accounts)
动态跟踪 `Free / Trial / Pro / Max` 全序列套餐状况。无需登录浏览器，随时监控最新订阅边界、当前运行时见底（Runtime Exhausted）、历史用量以及池绑定状况。

| 账号与号池管理视图 |
| :---: |
| ![Accounts](docs/images/preview-accounts.png) |

#### 2. 本地 OpenAI API 兼容中转 (OpenAI Relay)
集成 SSE 流式输出能力。您可以将自己购买或获取到的账号无缝接入类似 `ChatGPT-Next-Web`, `LobeChat`, `Cursor`, 甚至 `OpenAI SDK` 客户端。后端自带健康检测与故障倒换，前端全UI掌控模型映射。

| OpenAI Relay 控制台 |
| :---: |
| ![Relay](docs/images/preview-relay.png) |

#### 3. 流量用量统计面板 (Usage & Diagnostics)
全新设计的 **Usage Dashboard** 精确计算并聚合从您机器发往 Windsurf / OpenAI 的全部流通 Token 的数量以及大略转换的美金价值，全方位杜绝隐藏费用，更有完整历史流水审计明细。

| 数据用量与流水洞察 |
| :---: |
| ![Usage](docs/images/preview-usage.png) |

#### 4. 高级抓包与环境调试引擎 (Advanced MITM Config)
强大的 MITM 号池设置机制！从会话固化（Session Binding）、静默截获到高能协议体 Protobuf 的深度解析与截流。更支持直接抓取原始流水（Dump），方便二次排查分析。

| 核心层代理与策略配置 |
| :---: |
| ![Settings](docs/images/preview-settings.png) |

#### 5. 垃圾与残留清道夫 (Clean-Up Optimizer)
不再让海量 Cascade AI 对话数据和渲染缓存吃掉你珍贵的硬盘空间！轻轻一点即可完成各环节的精简化部署清理，重获新生。

| 磁盘瘦身优化 |
| :---: |
| ![Cleanup](docs/images/preview-cleanup.png) |

> ⚠️ *声明：当前仓库内上述预览图均为最新桌面端界面的脱敏展示图。我们永远不会窃取并上传任何账号池数据，全部本地化存储于 `settings.json`与 `accounts.json`。*

---

## 📦 下载发布包 | Download Releases

每次推送 `v*` 标签后，GitHub Actions 会自动构建并发布以下产物到 [Releases](https://github.com/seven7763/windsurf-tools/releases)：

| 文件 | 平台 | 说明 |
|------|------|------|
| `windsurf-tools-wails.exe` | Windows amd64 | 单文件，启动时默认请求管理员权限 |
| `windsurf-tools-wails-windows-amd64.zip` | Windows amd64 | Windows 单文件压缩包 |
| `windsurf-tools-wails-macos-intel-amd64.zip` | macOS Intel | 打包后的 `.app` 压缩包 |
| `windsurf-tools-wails-macos-apple-silicon-arm64.zip` | macOS Apple Silicon | 打包后的 `.app` 压缩包 |
| `SHA256SUMS.txt` | 全平台 | 所有发布文件的 SHA256 校验 |

> 本程序在 Windows 下默认请求管理员运行以实现完整的代理劫持（Hosts、CA安装配置）。请放心授予或采用受控模式运行。macOS 环境需要处理好初次的 Gatekeeper。

---

## 💻 运行环境 | Prerequisites 

### Windows
- Windows 10 / 11 `amd64` 
- [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) 依赖

### macOS
- 支持 Intel (`amd64`) 及 Apple Silicon (`arm64`) 双架构。由于使用跨平台 Webview UI，苹果系统亦可享用统一的视觉体验。

---

## 🧰 从源码构建 | Build from Source

#### 前置条件
- [Go](https://go.dev/dl/) 1.24.x
- [Node.js](https://nodejs.org/) 20+
- [Wails CLI v2](https://wails.io/docs/gettingstarted/installation)

```bash
git clone https://github.com/seven7763/windsurf-tools.git
cd windsurf-tools

# 安装前端依赖
cd frontend
npm install
cd ..

# 编译应用 (默认输出在 build/bin/ 下)
wails build
```

---

## ⚙️ 系统集成：服务化运转模式

支持基于 [kardianos/service](https://github.com/kardianos/service) 的无 UI 后台服务模式（纯 Daemon），使得你的工作环境能持久享受 OpenAI 中继及 MITM 打通福利！

`windsurf-tools-wails.exe install/start/stop`

---

## 📁 隐私与数据目录 | Privacy

应用核心配置目录：
- **Windows**：`%APPDATA%\WindsurfTools\`
- **macOS**：`~/.windsurf-tools/`（含 CA 证书 `ca/ca.pem`）

内部保存 `accounts.json`、`settings.json` 及全套 MITM 证书。**切勿向公共仓库提交这些文件。** 详见 [SECURITY.md](SECURITY.md)。

---

## 🔧 最近修复 | Recent Fixes

- **F20/F32 计费字段替换** — 修复原先只替换 api_key+JWT 不替换 UserID/TeamID 导致上游 auth 用号池账号但 billing 仍记登录用户的严重 Bug（`proxy_identity.go`）
- **macOS 26+ CA 信任** — 改用 Terminal.app 交互式 sudo 走 `security add-trusted-cert`，解决 osascript 无法完整授权的问题
- **单密码批量特权** — `hosts` / DNS flush / 端口 443 绑定合并进一次弹窗，不再多次输入密码
- **Clash TUN 模式兼容** — 自动维护 `Merge.yaml` hosts + DIRECT 规则，避免 TUN 接管后绕过 `/etc/hosts`
- **会话粘性 pool key** — 同一 Cascade conversation 稳定复用同一 pool key，避免 `Invalid Cascade session` 错误

---

## ⚠️ 免责声明 | Disclaimer

本项目仅供学习研究 Windsurf / Codeium 协议使用。使用本工具进行商业规避、批量滥用或违反 Windsurf/Codeium 服务条款的行为，相关责任由使用者自负。作者不鼓励、不支持任何违反目标服务 ToS 的用法。

---

## 📄 开源许可 | License
[MIT License](LICENSE)
