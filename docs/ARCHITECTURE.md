# Windsurf Tools — 架构文档

> 自动生成于 2026-03-24，基于完整代码审计。

---

## 1. 项目概览

Windsurf Tools 是一个 **Wails v2** 桌面应用（Go 后端 + Vue 3 前端），用于管理多个 Windsurf IDE 账号，实现：

- **号池管理**：批量导入（邮箱密码 / RefreshToken / API Key / JWT）、自动刷新凭证与额度
- **无感切号**：额度用尽时自动切到下一可用账号，写入 `windsurf_auth.json` + `config.json`
- **MITM 代理**：本地 TLS 反向代理，透明替换 gRPC 请求中的身份（API Key + JWT），多号轮换
- **OpenAI 兼容中转**：`/v1/chat/completions` 接口，复用 MITM 号池，支持流式 SSE
- **后台服务**：可注册为 Windows 服务 / macOS LaunchAgent，无界面运行

---

## 2. 目录结构

```
windsurf-tools-wails/
├── main.go                  # 入口：CLI (install/start/stop) / daemon / desktop
├── app.go                   # App 结构体、initBackend、startup/shutdown
├── app_accounts.go          # 账号 CRUD（GetAll/Delete/DeleteExpired/DeleteFree）
├── app_enrich.go            # 账号信息丰富化（JWT解析/GetPlanStatus/GetUserStatus）
├── app_import.go            # 批量导入（EmailPassword/RefreshToken/APIKey/JWT）
├── app_lifecycle.go         # MITM 环境清理、多实例检测、窗口激活
├── app_mitm.go              # MITM 代理控制（Start/Stop/Setup/Teardown）
├── app_patch.go             # Windsurf 补丁（查找安装路径、重启）
├── app_quota.go             # 自动刷新 Token/额度、热轮询、用尽切号
├── app_relay.go             # OpenAI 中转控制（Start/Stop/Status）
├── app_settings.go          # 设置读写 + 代理/自动刷新联动
├── app_switch.go            # 切号核心（手动/自动切号、预热、MITM轮换）
├── app_window.go            # 窗口布局（Toolbar 小条模式 / 主窗口）
├── service.go               # 后台服务（headlessProgram、日志轮转、状态）
├── tray.go / tray_stub.go   # 系统托盘（仅支持平台编译）
│
├── backend/
│   ├── models/
│   │   ├── account.go       # Account 结构体
│   │   └── settings.go      # Settings 结构体 + DefaultSettings
│   ├── store/
│   │   ├── store.go         # JSON 文件存储（accounts.json / settings.json）
│   │   └── account_conflict.go  # 去重判断（Email/APIKey/Token/RefreshToken）
│   ├── services/
│   │   ├── windsurf.go      # Windsurf API 客户端（Firebase Auth / gRPC / RegisterUser）
│   │   ├── proxy.go         # MITM 反向代理核心（号池、身份替换、额度检测、重试）
│   │   ├── proxy_identity.go    # protobuf 身份替换（ReplaceIdentityInBody）
│   │   ├── proxy_cert.go        # CA 证书生成 / 安装 / 卸载
│   │   ├── proxy_hosts.go       # hosts 文件劫持 / 恢复
│   │   ├── proxy_registry_windows.go  # Windows 注册表 ProxyOverride
│   │   ├── openai_relay.go      # OpenAI 兼容 API 中转服务器
│   │   ├── switch.go            # 切号文件操作（WriteAuthFile / GetCurrentAuth）
│   │   ├── codeium_config.go    # ~/.codeium/config.json API Key 注入
│   │   ├── chat_proto.go        # gRPC protobuf 构建/解析（chat 请求/响应）
│   │   ├── proto_dump.go        # protobuf 调试 dump
│   │   ├── patch.go             # Windsurf 安装路径检测、重启
│   │   ├── ide_profiles.go      # 独立窗口 profile 路径计算
│   │   └── ide_profile_bootstrap.go  # profile 初始化 + 配置拷贝
│   ├── utils/                   # 额度判断、计划匹配、时间计算
│   └── paths/                   # 跨平台配置目录解析
│
└── frontend/src/
    ├── App.vue              # 根组件（主界面 / ToolbarStrip 切换）
    ├── views/
    │   ├── Dashboard.vue    # 仪表板（额度概览、洞察）
    │   ├── Accounts.vue     # 账号列表（导入、切号、刷新）
    │   ├── Relay.vue        # MITM + OpenAI 中转面板
    │   └── Settings.vue     # 设置页
    ├── stores/              # Pinia stores (account/settings/mitm/system/mainView)
    ├── components/          # UI 组件（AccountCard/MitmPanel/ToolbarStrip）
    └── utils/               # 前端工具（导入解析、时区、图表）
```

---

## 3. 核心数据流

### 3.1 切号流程（文件模式）

```
用户点击「切号」/ 额度用尽自动触发
  │
  ▼
orderedSwitchCandidates()          ← 排序候选（新鲜优先、过期stale次之）
  │
  ▼
prewarmCandidates(top 3)           ← 并行刷新JWT+额度，写入store
  │
  ▼
重读 store 获取最新数据              ← 避免 prepareAccountForUsage 重复调API
  │
  ▼
prepareAccountForUsage(acc)        ← syncCredentials + enrichQuota + 验证
  │
  ▼
switchSvc.SwitchAccount()          ← WriteAuthFile (直写→tmp+rename→PowerShell)
  │                                   + verifyAuthFileWrite 回读验证
  ▼
InjectCodeiumConfig()              ← ~/.codeium/config.json 写入 API Key
  │
  ▼
mitmProxy.SwitchToKey()            ← MITM 代理同步到新号
  │
  ▼
applyPostWindsurfSwitch()          ← 协议URI刷新 + 可选重启Windsurf
```

### 3.2 MITM 代理请求流

```
Windsurf IDE  ──(hosts 劫持)──▶  127.0.0.1:443 (TLS)
                                     │
                                     ▼
                              handleRequest()
                              ├── 读取 protobuf body
                              ├── pickPoolKeyAndJWT()    ← 号池选key+JWT
                              ├── ReplaceIdentityInBody() ← 替换身份
                              └── 设 Authorization header
                                     │
                                     ▼
                              retryTransport.RoundTrip()
                              ├── 转发至上游 (ResolveUpstreamIP)
                              ├── 检测额度耗尽 → markExhausted + rotateKey + 重试
                              ├── 检测认证失败 → refreshJWT + 重试
                              └── 最多重试 3 次
                                     │
                                     ▼
                              handleResponse()
                              ├── 响应CT检查(优先响应头,回退请求头)
                              ├── 小包: buffered检查额度错误
                              ├── 流式: quotaStreamWatchBody 边转发边监测
                              ├── GetUserJwt: 捕获新JWT缓存
                              └── 更新 keyState (success/exhausted)
```

### 3.3 额度监控 & 自动切号

```
startAutoQuotaRefresh (5min)      ← 定期同步所有非当前号的额度
  │
  ├── refreshDueQuotas()          ← 按策略过滤到期账号
  │     ├── syncCredentials + enrichInfo（并行批次）
  │     └── 若当前号耗尽 → AutoSwitchToNext / rotateMitm
  │
  └── quotaHotPollLoop (12s)      ← 高频盯当前Windsurf会话
        ├── 读 windsurf_auth.json 匹配当前号
        ├── enrichQuotaOnly（仅拉额度，不RegisterUser）
        └── 耗尽 → 立即切号（12s冷却防抖）
```

---

## 4. 关键模块

### 4.1 Store (`backend/store/store.go`)

- **线程安全**：`sync.RWMutex` 保护所有读写
- **持久化**：`accounts.json` / `settings.json`，每次修改即刷盘
- **去重**：`AccountsConflict` 按 Email / APIKey / Token / RefreshToken 去重
- **向后兼容**：`load()` 检查旧 settings 缺失字段并填充默认值

### 4.2 MITM Proxy (`backend/services/proxy.go`)

| 组件 | 职责 |
|------|------|
| `PoolKeyState` | 每个 API Key 的运行时状态（健康/耗尽/冷却/计数） |
| `retryTransport` | 包装 http.Transport，额度耗尽自动轮转重试 |
| `quotaStreamWatchBody` | 包装响应Body，流式传输中实时检测额度耗尽关键字 |
| `jwtRefreshLoop` | 每4分钟强制刷新所有key的JWT |
| `ResolveUpstreamIP` | 动态DNS解析+5分钟缓存，过滤127.x（hosts劫持），失败回退硬编码 |

### 4.3 OpenAI Relay (`backend/services/openai_relay.go`)

- 监听 `127.0.0.1:8787`，支持 Bearer token 鉴权
- `/v1/chat/completions`: 构建 gRPC protobuf → 转发上游 → 解析响应 → 转为 SSE 或 JSON
- 复用 MITM 号池 (`pickPoolKeyAndJWT`)，额度耗尽自动轮转重试（最多3次）

### 4.4 切号 (`backend/services/switch.go`)

**WriteAuthFile 三级降级写入**（兼容管理员 Windsurf 锁定文件）：
1. 直接 `os.WriteFile` → 成功则回读验证
2. 失败 → 写临时文件 `.tmp` + `os.Rename` 覆盖
3. 仍失败（Windows）→ PowerShell `[IO.File]::WriteAllText` 强制写入
4. 写入后 `verifyAuthFileWrite` 回读确认 token 一致

### 4.5 凭证刷新优先级

```
syncAccountCredentialsWithService:
  1. WindsurfAPIKey → GetJWTByAPIKey (重试1次, 500ms间隔)
  2. RefreshToken   → Firebase RefreshToken
  3. Email+Password → Firebase SignInWithPassword
```

---

## 5. 已知问题 & 设计权衡

### 5.1 本轮修复的 Bug

| # | 严重度 | 描述 | 修复 |
|---|--------|------|------|
| 1 | **HIGH** | `handleResponse` 用请求CT判断proto，导致响应体额度检测失效 | 优先响应CT，回退请求CT |
| 2 | **HIGH** | MITM proxy transport 缺 TLS ALPN `NextProtos`，HTTP/2协商失败 | 添加 `["h2","http/1.1"]` |
| 3 | **HIGH** | `prewarmCandidates` 后未重读store，`prepareAccountForUsage` 重复调API | 预热后重读store构建freshMap |
| 4 | **HIGH** | `WriteAuthFile` 管理员Windsurf锁文件时写入失败 | 三级降级+PowerShell+回读验证 |
| 5 | **HIGH** | 切号只写auth不同步config.json，Windsurf重读后仍用旧key | SwitchAccount+AutoSwitch均同步InjectCodeiumConfig |
| 6 | **MEDIUM** | `codeium config.json` 同样可能被锁定 | `robustWriteFile` 统一降级策略 |
| 7 | **HIGH** | `LoginWithEmail` 用 `fmt.Sprintf` 拼 JSON，密码含 `"` 时 JSON 断裂 | 改用 `json.Marshal` 正确转义 |
| 8 | **MEDIUM** | Store `saveAccounts/saveSettings` 直写文件，崩溃时可能损坏 JSON | 改为 `atomicWriteFile`（tmp+rename，失败回退直写） |
| 9 | **HIGH** | gRPC status 9 (FAILED_PRECONDITION) 额度用尽未触发切号：(a) body 为空时跳过分类 (b) 未读 Trailer (c) status 9 未识别为 quota | 三处修复：移除空 body 跳过、读 Header+Trailer、status 9+quota 关键词→quota |

### 5.2 待观察

| 项 | 说明 |
|----|------|
| **`.bak.*` 文件累积** | `WriteAuthFile` 每次切号创建带时间戳的备份，长期运行会累积。建议保留最近N份 |
| **`refreshAllTokens` 不处理邮箱密码** | 只刷新 APIKey/RefreshToken 账号的JWT；邮箱密码账号需 quota refresh 或切号时才会登录 |
| **`permission_denied` → auth** | Windsurf API 用此表示JWT过期，故意归类为auth触发刷新。若上游语义变化需重新评估 |

---

## 6. 构建 & 运行

```bash
# 开发
wails dev

# 构建
wails build

# 安装为 Windows 服务
windsurf-tools.exe install
windsurf-tools.exe start

# CLI 控制
windsurf-tools.exe stop / restart / uninstall
```

---

## 7. 配置文件路径

| 平台 | 路径 |
|------|------|
| Windows | `%APPDATA%\WindsurfTools\` |
| macOS | `~/Library/Application Support/WindsurfTools/` |
| Linux | `~/.config/WindsurfTools/` |

文件：
- `accounts.json` — 号池数据
- `settings.json` — 全局设置
- `ca.crt` / `ca.key` — MITM CA 证书
- `desktop-runtime.log` — 桌面会话日志
- `background-service.log` — 后台服务日志

Windsurf auth 文件：
- Windows: `%APPDATA%\.codeium\windsurf\config\windsurf_auth.json`
- macOS: `~/.codeium/windsurf/config/windsurf_auth.json`
