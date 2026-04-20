# MITM 代理接口路由文档

## 概述

本应用通过系统 Hosts 劫持 + 本地 TLS MITM 代理拦截 Windsurf IDE 的 gRPC/Connect 请求，
实现号池轮换（身份替换）、Forge 伪装和静态缓存拦截。

**劫持域名**（Hosts 指向 127.0.0.1）：
- `server.self-serve.windsurf.com`
- `server.codeium.com`

**上游真实 IP**：`34.49.14.144`（DNS 解析缓存，2 分钟 TTL）

---

## 请求处理流程

```
Windsurf IDE → Hosts 劫持 → 本地 MITM (127.0.0.1:443)
  ├─ 静态缓存拦截？ → 直接返回本地 .bin 文件（不经过上游）
  └─ 否 → ReverseProxy 转发到上游
       ├─ 非 protobuf 请求 → 透传（不读 body）
       ├─ protobuf 但无 conversation_id 可能的路径 → 透传（不读 body）
       ├─ 有 conversation_id 可能但实际无 conv_id 且非 Chat 路径 → 透传
       └─ Chat/Cortex/Trajectory 路径 → 身份替换（中转）
```

---

## 接口分类

### 1. 静态缓存拦截（不经过上游，直接返回本地文件）

| 接口路径 | 缓存文件名 | 说明 |
|---------|-----------|------|
| `GetUserStatus` | `GetUserStatus.bin` | 用户状态查询 |
| `GetModelStatuses` | `GetModelStatuses.bin` | 模型列表查询 |
| `GetCommandModelConfigs` | `GetCommandModelConfigs.bin` | 命令模型配置 |

> 需在设置中开启静态缓存，并配置缓存目录。
> 开启后上述接口完全不走上游，直接从本地 `.bin` 文件返回预录制的响应。
> **注意**：这些接口即使未开启静态缓存，请求侧也是透传（不替换身份），
> 只是会走一次上游往返。静态缓存的作用是省掉这次往返。

---

### 2. Forge 伪装（经过上游，但篡改响应）

| 接口路径 | 伪装内容 | 说明 |
|---------|---------|------|
| `GetUserStatus` | 伪造用户状态、额度、订阅类型 | 伪装为 Pro/Team 用户 |
| `GetPlanStatus` | 伪造计划信息、计费周期 | 延长订阅期限、伪造额度 |

> Forge 在 `handleResponse` 阶段修改上游返回的 protobuf 响应。
> 仅当 Forge 开启 + 响应状态码 200 时生效。
> 若静态缓存同时开启，`GetUserStatus` 会被静态缓存拦截，Forge 不生效。

**Forge 伪装字段**：
- `FakeCredits` — 伪造剩余额度
- `FakeCreditsPremium` — 伪造 Premium 额度
- `FakeSubscriptionType` — 伪造订阅类型（Pro/Team/Enterprise）
- `FakeCreditsUsed` — 伪造已用额度
- `ExtendYears` — 延长订阅到期时间（年）

---

### 3. 身份替换中转（读取 body，替换 API Key + JWT）

所有 `mayHaveConversationID = true` 的接口都会进入身份替换流程，
但具体行为因路径类型和是否携带 `conversation_id` 而异。

#### 3.1 GetChatMessage / GetChatMessageBurst（主聊天接口）

**路径特征**：流式 gRPC-Connect 协议，Content-Type `application/connect+proto`，
携带 `conversation_id`（UUID 格式，protobuf 字段或 body 正则匹配）。

**`isChatPath = true`** · **`isBilling = true`** · **`mayHaveConversationID = true`**

---

**请求侧完整流程**：

1. **读取 body**：因为是 `mayHaveConversationID` 路径，读取完整请求 body
2. **提取 conversation_id**：`ExtractConversationIDFromBody` 在解压后的 payload 中扫描 UUID 格式字符串
3. **会话路由决策**：
   - **有 conv_id** → `pickPoolKeyForSession(convID)` 查找已有会话绑定，返回绑定的 Key+JWT
   - **无 conv_id（新对话首条消息）** → `pickKeyForNewConversation()` 分配 Key，推入 `pendingNewConvKeys` 待绑定队列；
     当同一对话的第二条消息到达（携带 conv_id）时，从队列弹出匹配的 Key 完成绑定
4. **提取会话标题**：`ExtractSessionTitleHint` 从 protobuf F3 repeated → F3=content 提取用户首条消息前 40 字符作为标题提示
5. **身份替换**：`ReplaceIdentityInBody(body, newKey, newJWT, randFP, fp)`
   - 解压 body → 替换 F1 metadata 子消息中的 `sk-ws-` API Key + `eyJ` JWT
   - 若原始请求无 API Key，检测 F1=ide_name + F2=ide_version 结构后注入 Key
   - 每个 Key 独立的 session ID + 设备指纹（防 session 级限速）
   - Trial/Free Key 额外随机化指纹（`isTrialOrFreeKey` → `randFP=true`）
6. **替换 Authorization 头**：`Authorization: Bearer <poolJWT>`
7. **设置追踪头**：`X-Pool-Key-Used: <poolKey>`（响应侧追踪用，发送前删除）
8. **安全回退**：无可用 Key 或 JWT 未就绪 → 透传原始请求，绝不替换身份

---

**响应侧完整流程**（`isBilling = true`，核心计费接口）：

响应侧有 **两层错误检测**：`retryTransport`（请求重试层）和 `handleResponse`（响应后处理层）。

##### 第一层：retryTransport（透明自动重试）

在 `RoundTrip` 中拦截上游响应，根据错误类型决定是否自动重试：

| 错误类型 | kind | 行为 |
|---------|------|------|
| 额度耗尽 | `quota` | `markRuntimeExhaustedAndRotate` 切号 → 用新 Key 重新构造请求重试 |
| 普通限速 | `rate_limit` | `markRateLimitedAndRotate` 短冷却+切号 → 重试；已有 conv_id 的对话保持粘性透传 |
| Trial 全局限速 | `global_rate_limit` | 标记冷却+轮转 → **透传给 IDE**（不重试，因为换号后 Cascade session 不匹配） |
| 认证失败 | `auth` | 先尝试 `refreshJWTForKey` 刷新 JWT 重试；失败则 `rotateAfterAuthFailure` 切号重试 |
| 权限拒绝 | `permission` | `disableKeyAndRotate` 禁用 Key + 切号重试 |
| 上游不可达 | `unavailable` | 同一 Key 重试（非 Key 问题） |
| Invalid Cascade Session | `auth` | 剥离 `conversation_id` 后重试（最多 1 次） |
| 其他错误 | `internal`/`grpc` | 记录失败，不重试 |

**重试预算**：`defaultReplayBudget = 2`（最多 2 次自动重试）

**已有对话的粘性策略**：
- 限速冷却 → 保持粘性透传（冷却 120s 后自动恢复，切号会导致 Invalid Cascade session）
- 额度耗尽 → 允许迁移（Key 不会自动恢复，`pickPoolKeyForSession` 会检测 `RuntimeExhausted` 分配新 Key）
- 认证失败 → 保持 session 粘性（同一 Key 刷新 JWT 后重试）

##### 第二层：handleResponse（响应后处理）

`retryTransport` 透传后的响应进入 `handleResponse`，根据响应大小和类型分三种处理路径：

**路径 A — 小包缓冲检测**（`shouldCheckBuffered`）：
- 条件：`ContentLength ≥ 0 && < 5000`，或 `StatusCode ≥ 400`，或 HTTP 200 + JSON Content-Type（协议异常）
- 行为：读取完整 body → `ParseConnectEOS` 解析 Connect EOS 帧或 JSON 错误 → `ClassifyConnectError` 分类
- 错误处理：
  - `quota` → 标记 `isExhausted`（后续触发 `onKeyExhausted` 回调，App 层刷新额度+同步号池）
  - `global_rate_limit` → `markRateLimitedAndRotate` 轮转
  - `rate_limit` → `markRateLimitedAndRotate` 轮转
  - `auth`/`permission` → `rotateAfterAuthFailure` 轮转
  - 其他 → `recordUpstreamFailure` 记录
- 用量记录：`recordMitmUsage`

**路径 B — 流式响应监控**（`shouldWatchStream`）：
- 条件：`isBilling && StatusCode == 200 && !shouldCheckBuffered`（大包流式响应）
- 行为：用 `ConnectStreamWatcher` 包装 response body，逐帧扫描
  - 数据帧（flag=0x00）→ 透传给 IDE
  - EOS trailer 帧（flag & 0x02）→ 解析 JSON 错误，触发 `onError` 回调
  - 流正常结束（无 EOS 错误）→ 触发 `onSuccess` 回调
- 错误处理（同路径 A 分类）：
  - `quota` → `markRuntimeExhaustedAndRotate` 轮转
  - `global_rate_limit` → `markRateLimitedAndRotate` 轮转 + `recordUpstreamFailure`
  - `rate_limit` → `markRateLimitedAndRotate` 轮转
  - `auth`/`permission` → `rotateAfterAuthFailure` 轮转
  - 其他 → `recordUpstreamFailure` 记录
- 成功回调：`recordSuccess`（重置连续错误计数）+ `recordMitmUsage`（记录 completionTokens）

**路径 C — 简单成功**：
- 条件：`isBilling && StatusCode == 200` 且不满足 A/B
- 行为：`isSuccess = true` + `recordMitmUsage`

---

**额外响应处理**：

- **额度耗尽后续**：`isExhausted = true` 时，在 `handleResponse` 末尾调用 `onKeyExhausted` 回调 →
  App 层 `handleMitmKeyAccessDenied` 持久化账号额度状态 + `syncMitmPoolKeys` 同步号池
- **成功计数**：`isSuccess && isBilling` → `recordSuccess`（重置 `ConsecutiveErrs`）
- **Debug Dump**：开启后 dump GetChatMessage 请求/响应的 protobuf 字段树到文件
- **全量抓包**：开启后记录所有请求/响应到 JSONL + body 文件
- **流量日志**：`shouldCaptureTrafficPath` 匹配时记录请求/响应摘要 + body dump

---

#### 3.2 GetCompletions（代码补全接口）

**路径特征**：流式 gRPC-Connect 协议，Content-Type `application/connect+proto`，
可能携带 `conversation_id`（补全请求通常属于已有对话）。

**`isChatPath = true`** · **`isBilling = true`** · **`mayHaveConversationID = true`**

---

**请求侧**：与 GetChatMessage 完全相同的身份替换流程。

**响应侧**：与 GetChatMessage 完全相同的双层错误检测 + 自动轮转逻辑。

**与 GetChatMessage 的差异**：
- 请求 body 更小（代码补全上下文 vs 完整对话），通常走小包缓冲路径（路径 A）
- 流式响应也可能走路径 B（长补全结果）
- 无 conv_id 的新补全请求同样走 `pickKeyForNewConversation` 分配 Key
- Debug Dump 仅对 `GetChatMessage` 路径触发，GetCompletions 不 dump protobuf 字段树

#### 3.3 Cortex.*（Cortex 相关接口）

**路径特征**：gRPC-Connect，可能携带 `conversation_id`

**请求侧**：
1. 读取 body，提取 `conversation_id`
2. **有 conv_id** → 按 conv_id 查找会话绑定，进行身份替换
3. **无 conv_id** → **透传**（不替换身份，直接转发原始请求）

**响应侧**：
- `isBilling = false`，不检测额度耗尽/限速等错误
- 不记录用量统计
- 仅做基础响应转发

#### 3.4 Trajectory.*（轨迹/会话管理接口）

**路径特征**：gRPC-Connect，可能携带 `conversation_id`

**请求侧**：
1. 读取 body，提取 `conversation_id`
2. **有 conv_id** → 按 conv_id 查找会话绑定，进行身份替换
3. **无 conv_id** → **透传**（不替换身份，直接转发原始请求）

**响应侧**：
- `isBilling = false`，不检测额度耗尽/限速等错误
- 不记录用量统计
- 仅做基础响应转发

---

#### 共同的身份替换细节

**替换内容**（`ReplaceIdentityInBody`）：
- 替换 protobuf F1 metadata 子消息中的 `sk-ws-` API Key
- 替换 `eyJ` 开头的 JWT token
- 若原始请求无 API Key，检测 F1=ide_name + F2=ide_version 结构后注入 Key
- 每个 Key 独立的 session ID + 设备指纹（防 session 级限速）
- Trial/Free Key 额外随机化指纹

**会话绑定**：
- 每个 `conversation_id` 绑定一个号池 Key（sticky routing）
- 同一对话的所有请求走同一个 Key
- 首条消息（无 conv_id）分配 Key 后推入待绑定队列
- 第二条消息到达时从队列弹出匹配的 Key 完成绑定
- 额度耗尽时自动轮转到下一个可用 Key
- 迁移的会话标记 `Migrated`（当前代码中剥离 conv_id 逻辑已注释）

**安全回退**：
- 号池无可用 Key → 透传原始请求（不替换身份）
- JWT 未就绪 → 透传原始请求（绝不替换身份）
- `ReplaceIdentityInBody` 替换失败 → 透传原始 body

---

### 4. 透传（不读 body，不修改，零延迟）

| 条件 | 说明 |
|------|------|
| 非 protobuf Content-Type | 非 `application/proto` / `application/grpc` 请求 |
| `mayHaveConversationID` 返回 false | 身份/状态类接口（请求侧始终透传） |
| 有 conv_id 可能但实际无 conv_id 且非 Chat | Cortex/Trajectory 无会话 ID 的请求 |
| 号池无可用 Key 或 JWT 未就绪 | 安全回退：不替换身份，直接透传 |

**始终透传的身份/状态接口**（`mayHaveConversationID = false`，请求侧不读 body、不替换身份）：
- `GetUserStatus` — 开启静态缓存时直接返回本地文件；否则透传到上游（响应可能被 Forge 篡改）
- `GetPlanStatus` — 透传到上游（响应可能被 Forge 篡改）
- `Ping`
- `GetProfileData`
- 其他不含 `GetChatMessage`/`GetCompletions`/`Cortex`/`Trajectory` 的 gRPC 接口

> 这些接口的请求侧永远不替换身份（API Key / JWT），
> 因为它们不携带 `conversation_id`，无需号池路由。
> Forge 伪装仅在**响应侧**修改上游返回内容，不影响请求透传行为。

---

### 5. OpenAI Relay 中转（独立 HTTP 服务）

监听端口由 `openai_relay_port` 设置控制（默认 8787）。

| 路由 | 方法 | 说明 |
|------|------|------|
| `/v1/chat/completions` | POST | OpenAI 聊天补全 → 转换为 Windsurf gRPC `GetChatMessage` |
| `/v1/models` | GET | 列出可用模型 |
| `/v1/messages` | POST | Anthropic Messages API → 转换为 Windsurf gRPC（Claude Code 兼容） |
| `/v1/usage` | GET/DELETE | 用量记录查询/清除 |
| `/v1/usage/summary` | GET | 用量汇总 |
| `/health` | GET | 健康检查 |

> Relay 使用号池中的 API Key + JWT 发起上游 gRPC 请求，
> 将 OpenAI/Anthropic 格式转换为 Windsurf Connect 协议。
> 支持 SSE 流式响应。

#### 5.1 /v1/chat/completions（OpenAI 兼容）

**请求格式**：标准 OpenAI Chat Completions JSON（`model` + `messages` + `stream`）

**处理流程**：
1. 解析 JSON 请求，估算 prompt tokens
2. `pickPoolKeyAndJWT()` 从号池取 Key+JWT
3. `BuildChatRequestWithModel` 构建 Windsurf Connect protobuf 请求
4. `WrapGRPCEnvelope` 添加 gRPC 5 字节帧头
5. `sendGRPC` 直连上游 `34.49.14.144` 发送 Connect 请求
6. 错误自动重试（最多 `maxRetry` 次）：
   - `quota` → `markRuntimeExhaustedAndRotate` + 重试
   - `global_rate_limit` → 放弃重试，返回 429
   - `rate_limit` → `markRateLimitedAndRotate` + 重试
   - `auth` → `rotateAfterAuthFailure` 或 `refreshJWTForKey` + 重试
7. 成功后根据 `stream` 参数：
   - `stream=true` → `streamResponse` SSE 流式输出
   - `stream=false` → `blockingResponse` 一次性 JSON
8. `recordUsage` 记录用量

#### 5.2 /v1/messages（Anthropic 兼容）

**处理流程**：与 `/v1/chat/completions` 类似，但：
- 请求格式为 Anthropic Messages API（`model` + `messages` + `system`）
- 响应格式为 Anthropic SSE 事件流（`message_start`/`content_block_delta`/`message_stop`）
- 自动转换 Anthropic role 到 Windsurf 对话格式
- 支持 Claude Code 客户端

#### 5.3 Relay 鉴权

- 可通过 `openai_relay_secret` 设置密钥
- 请求需携带 `Authorization: Bearer <secret>`
- 未设置密钥时无需鉴权（本地使用）
- 内置 CORS 支持（`Access-Control-Allow-Origin`）

---

### 6. GetUserJwt 响应捕获（隐式透传 + 响应侧截取）

`GetUserJwt` 接口是一个特殊的透传接口：

- **请求侧**：`mayHaveConversationID = false`，走快速透传路径
- **响应侧**：如果有 `X-Pool-Key-Used` 头（即身份替换过的请求），从响应 body 中提取 JWT 并缓存
  - `ExtractJWTFromBody` 解析 protobuf 响应中的 JWT token
  - `updateJWT(usedKey, jwt)` 更新号池 Key 对应的 JWT 缓存
  - 用于后续请求的 `Authorization` 头替换

> 这是号池 JWT 自动续期的核心机制：IDE 定期调用 `GetUserJwt` 获取新 JWT，
> 代理在响应侧截取并缓存，确保号池 Key 的 JWT 始终有效。

---

## 号池 Key 状态管理

### PoolKeyState 结构

每个号池 Key 维护一个 `PoolKeyState`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Healthy` | bool | 健康状态（可用） |
| `Disabled` | bool | 永久禁用（权限拒绝等） |
| `RuntimeExhausted` | bool | 额度耗尽（不靠冷却自动恢复） |
| `CooldownUntil` | time | 冷却截止时间 |
| `ConsecutiveErrs` | int | 连续错误计数 |
| `RequestCount` | int | 请求计数 |
| `SuccessCount` | int | 成功计数 |
| `TotalExhausted` | int | 累计耗尽次数 |
| `SessionID` | string | Per-key 稳定 UUID v4（F32 字段，创建时生成） |
| `DeviceHash` | string | Per-key 稳定 hex hash（F31/F27 字段，创建时生成） |
| `Plan` | string | 计划类型（Pro/Trial/Free/Team 等） |
| `JWT` | []byte | 缓存的 JWT token |

### 状态转换

```
                 创建
                  │
                  ▼
            ┌─────────┐
            │ Healthy  │ ← recordSuccess / 冷却到期
            └─────────┘
               │  │  │
    ┌──────────┘  │  └──────────┐
    ▼             ▼              ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│ Cooldown │  │Exhausted │  │ Disabled │
│(限速/瞬态)│  │(额度耗尽)│  │(权限拒绝)│
└──────────┘  └──────────┘  └──────────┘
  120s 恢复    需手动/API      永久禁用
               解除
```

### 冷却时间常量

| 常量 | 值 | 说明 |
|------|-----|------|
| `keyCooldownSec` | 600s (10min) | 额度耗尽冷却 |
| `rateLimitCooldownSec` | 120s (2min) | 限速冷却 |
| `globalTrialRateLimitBackoffSec` | 60s (1min) | 全局 Trial 限速退避 |
| `defaultReplayBudget` | 2 | 最大重试次数 |
| `jwtRefreshMinutes` | 4min | JWT 刷新间隔 |

### isAvailable 可用性判断

```
Disabled = true        → 不可用（永久）
Healthy = true         → 可用
RuntimeExhausted = true → 不可用（不靠冷却恢复，需 recordSuccess/ClearKeyExhausted）
time.Now > CooldownUntil → 恢复为 Healthy（瞬态错误冷却到期）
time.Now ≤ CooldownUntil → 不可用（冷却中）
```

---

## 全局 Trial 限速退避

当检测到 `global rate limit for trial users` 关键词时：

1. `markGlobalTrialRateLimit()` 设置退避截止时间（当前时间 + 60s）
2. 退避期间 `pickPoolKeyForSession` 和 `pickKeyForNewConversation` 优先跳过 Trial/Free Key
3. 仅在无 Pro/Team Key 可用时回退使用 Trial Key（`fallbackKey`）
4. 退避到期后自动恢复正常选择逻辑

**判断逻辑**：
- `isTrialOrFreeKey` → Plan 为空/Trial/Free 的 Key（保守策略：未知 Plan 视为 Trial）
- `isProOrTeamKey` → 取反

---

## 错误分类体系

### Connect 协议错误检测

Windsurf 使用 **Connect 协议**（非标准 gRPC）：

- **流式端点**（GetChatMessage/GetCompletions）：HTTP 200 + Connect frames
  - 数据帧：`flag=0x00` + 4 字节 big-endian 长度 + protobuf payload
  - EOS trailer 帧：`flag & 0x02` + 4 字节长度 + JSON `{"error":{"code":"xxx","message":"yyy"}}`
- **非流式端点**：HTTP 4xx + JSON body `{"code":"xxx","message":"yyy"}`
- **协议异常**：HTTP 200 + `Content-Type: application/json`（应为 `connect+proto`）

### ClassifyConnectError 优先级

| 优先级 | Connect code | 关键词匹配 | 映射到 kind |
|--------|-------------|-----------|------------|
| 0 | 任意 | `global` + `rate` + `limit` | `global_rate_limit` |
| 1 | 任意 | `rate limit` / `message limit` / `too many requests` 等 | `rate_limit` |
| 1 | `permission_denied` | 无 credit 关键词 | `permission` |
| 2 | `resource_exhausted` | 或含 credit 关键词 | `quota` |
| 3 | `failed_precondition` | 含 `quota`/`usage`/`credits` | `quota` |
| 3 | `failed_precondition` | 其他（如 Invalid Cascade session） | `grpc` |
| 4 | `unauthenticated` | — | `auth` |
| 5 | `unavailable` | 或含 `provider unreachable` | `unavailable` |
| 6 | `internal` | — | `internal` |
| — | 其他 | — | `grpc` |

### classifyUpstreamFailure（gRPC 回退）

当 Connect EOS 解析失败时，回退到 gRPC header + body text 检测：

| gRPC status | 关键词 | 映射到 kind |
|-------------|--------|------------|
| — | `rate limit` 系列关键词 | `rate_limit` |
| 8 (RESOURCE_EXHAUSTED) | 或 quota 关键词 | `quota` |
| 9 (FAILED_PRECONDITION) | 含 `quota`/`usage`/`credits` | `quota` |
| 16 (UNAUTHENTICATED) | 或 `unauthenticated` | `auth` |
| — | `permission_denied` JSON 格式 | `auth` |
| 14 (UNAVAILABLE) | 或 `provider unreachable` | `unavailable` |
| 13 (INTERNAL) | 或 `internal server error` | `internal` |
| 7 (PERMISSION_DENIED) | 或 `forbidden` | `permission` |
| 非 0 非空 | — | `grpc` |

### 额度耗尽关键词（`isQuotaExhaustedText`）

```
resource_exhausted, not enough credits, daily usage quota has been exhausted,
weekly usage quota has been exhausted, usage quota is exhausted,
included usage quota is exhausted, quota exhausted, daily_quota_exhausted,
weekly_quota_exhausted, purchase extra usage
+ (failed_precondition) && (quota|usage|credits)
```

### 限速关键词（`isRateLimitText`）

```
rate limit exceeded, rate limit error, rate limit, rate_limit,
global rate limit, over their global rate limit,
all api providers are over, message limit, limit will reset,
too many requests, try again in about an hour,
upgrade to pro for higher limits, higher limits,
add-credits, no credits were used
```
