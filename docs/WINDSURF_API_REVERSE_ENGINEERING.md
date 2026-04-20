# Windsurf IDE API 逆向分析报告

> 基于全量 MITM 流量抓包分析（2026-04-10），共捕获 765+ 条 JSONL 记录

---

## 1. 协议概述

| 项目 | 值 |
|------|------|
| **传输协议** | HTTP/2 over TLS |
| **RPC 框架** | Connect Protocol (Buf Connect) v1 |
| **序列化** | Protocol Buffers (`application/connect+proto`, `application/proto`) |
| **压缩** | gzip（请求 body 和流式 frame 均压缩） |
| **客户端** | `connect-go/1.18.1 (go1.26.1)` |
| **服务端域名** | `server.codeium.com` (实际 IP: `34.49.14.144`) |
| **Web 后端** | `web-backend.windsurf.com`（仅用于 JSON API） |

### Connect Protocol 帧格式

- **Unary 请求/响应**：body 直接是 protobuf（可能 gzip 压缩）
- **Streaming 响应**：每帧 = `flags(1B)` + `payload_len(4B, big-endian)` + `payload`
  - `flags & 0x01` = 压缩（gzip）
  - `flags & 0x02` = Trailer 帧（JSON 格式，包含 grpc-status）
- **23 字节响应**：空成功响应（`\x00\x00\x00\x00\x12` + 空 proto + trailer）

---

## 2. API 端点清单

### 2.1 核心 AI 服务 (`exa.api_server_pb.ApiServerService`)

| 端点 | 方法 | 请求大小 | 响应大小 | 模式 | 说明 |
|------|------|----------|----------|------|------|
| **GetChatMessage** | POST | 30-94 KB | 流式 | Server Streaming | Cascade 对话核心，流式返回 AI 生成内容 |
| **GetCompletions** | GET/POST | 0 B | 0 B | Unary | 代码自动补全（WebSocket 升级） |
| **GetCommandModelConfigs** | POST | ~1.9 KB | ~290 B | Unary | 获取可用模型配置和参数 |
| **GetModelStatuses** | POST | ~1.7 KB | ~23 B | Unary | 模型可用状态查询 |
| **GetStatus** | POST | ~1.9 KB | ~26 B | Unary | 服务状态检查 |
| **GetDefaultWorkflowTemplates** | POST | ~963 B | ~23 B | Unary | 获取工作流模板 |
| **CheckUserMessageRateLimit** | POST | ~1.7 KB | ~33 B | Unary | 消息频率限制检查 |
| **Ping** | POST | 0 B | 23 B | Unary | 心跳（每 ~30s） |

### 2.2 遥测/分析服务

| 端点 | 方法 | 请求大小 | 频率 | 说明 |
|------|------|----------|------|------|
| **RecordCortexTrajectoryStep** | POST | 1.8-8.9 KB | 极高（315次/30分钟） | Cascade 每步操作轨迹上报 |
| **RecordCortexGeneratorMetadata** | POST | 5-83 KB | 每次 AI 生成后 | AI 生成元数据（含完整 prompt） |
| **RecordAnalyticsEvent** | POST | 31-548 B | 高 | 产品分析事件 |
| **RecordAsyncTelemetry** | POST | 2-2.4 KB | 中 | 异步遥测数据 |
| **RecordEvent** | POST | 2.3-2.4 KB | 低 | 通用事件记录 |
| **RecordGitTelemetry** | POST | ~1 KB | 低 | Git 操作遥测 |
| **RecordProfilingData** | POST | **396 KB** | 极低 | 性能 profiling 数据（最大单请求） |
| **RecordTrajectorySegmentAnalytics** | POST | ~91 KB | 极低 | 轨迹段分析 |
| **RecordCortexExecutionMetadata** | POST | ~7 KB | 极低 | Cortex 执行元数据 |

### 2.3 认证/用户服务

| 端点 | 服务 | 请求大小 | 响应大小 | 说明 |
|------|------|----------|----------|------|
| **GetUserJwt** | `AuthService` | 160-462 B | ~900 B | JWT 获取/刷新 |
| **GetUserStatus** | `SeatManagementService` | 0-462 B | ~8 KB (gzip) | 用户状态 + 模型列表 + 额度 |
| **GetProfileData** | `SeatManagementService` | ~105 B | ~154 B | 用户身份信息 |

### 2.4 产品分析

| 端点 | 服务 | 说明 |
|------|------|------|
| **RecordAnalyticsEvent** | `ProductAnalyticsService` | 产品使用行为分析 |

---

## 3. 认证机制

### 3.1 API Key 认证

- **格式**：`sk-ws-01-{base64}` (约 80 字符)
- **传输**：通过 `Authorization` header（但抓包中显示由 MITM 代理注入到 `X-Pool-Key-Used`）
- **用途**：所有 API 调用的主要身份凭证

### 3.2 JWT Token

- **获取**：`GetUserJwt` 端点
- **格式**：标准 JWT (HS256)
- **Payload 结构**：

```json
{
  "api_key": "8f03cba2-dec3-40c7-a355-9bfccd207b25",
  "auth_uid": "4oTaRyS1Ilbck8LPMkgnU2kqnMC3",
  "email": "user@example.com",
  "name": "User Name",
  "exp": 1775756215,
  "pro": false,
  "team_id": "6071ca2c-b21a-4e92-9d90-fa9d3bab8db7",
  "team_status": "USER_TEAM_STATUS_APPROVED",
  "teams_tier": "TEAMS_TIER_PRO",
  "team_config": "{...}",
  "max_num_premium_chat_messages": 0,
  "disable_cli": false,
  "disable_codeium": false,
  "windsurf_pro_trial_end_time": "2026-04-05T10:49:39Z"
}
```

### 3.3 Team Config（嵌套 JSON）

```json
{
  "allowMcpServers": true,
  "allowAutoRunCommands": true,
  "maxUnclaimedSites": 5,
  "allowAppDeployments": true,
  "allowSandboxAppDeployments": true,
  "maxNewSitesPerDay": 5,
  "allowBrowserExperimentalFeatures": true,
  "allowVibeAndReplace": true,
  "allowCodemapSharing": "enabled",
  "maxCascadeAutoExecutionLevel": "CASCADE_COMMANDS_AUTO_EXECUTION_EAGER",
  "allowArenaMode": true
}
```

---

## 4. 模型列表（从 GetUserStatus 响应提取）

### 4.1 Claude 系列

| 模型 ID | 显示名 | 变体 |
|---------|--------|------|
| `claude-opus-4-6-thinking` | Claude Opus 4.6 Thinking | 标准/1M/Fast |
| `claude-opus-4-6` | Claude Opus 4.6 | 标准/1M/Fast |
| `claude-sonnet-4-6-thinking` | Claude Sonnet 4.6 Thinking | 标准/1M |
| `claude-sonnet-4-6` | Claude Sonnet 4.6 | 标准/1M |
| `claude-opus-4.5` | Claude Opus 4.5 | Thinking |
| `claude-sonnet-4` | Claude Sonnet 4 | Thinking/BYOK |
| Claude Haiku 4.5 | | |

### 4.2 GPT 系列

| 模型 ID | 显示名 | Thinking 级别 |
|---------|--------|---------------|
| `gpt-5-4-*` | GPT-5.4 | No/Low/Medium/High/XHigh + Fast 变体 |
| `gpt-5-4-mini-*` | GPT-5.4 Mini | Low/Medium/High/XHigh |
| `gpt-5-3-codex-*` | GPT-5.3-Codex | Low/Medium/High/XHigh + priority |
| `gpt-5.2-*` | GPT-5.2 | No/Low/Medium/High/XHigh + Fast |
| `gpt-5.1-codex-*` | GPT-5.1-Codex | Mini/Max + Low/Medium/High |
| `gpt-5-*` | GPT-5 | Low/Medium/High |
| `gpt-5p4` | GPT-5.4 (alias) | |

### 4.3 Gemini 系列

| 模型 ID | 显示名 |
|---------|--------|
| `gemini-3-1-pro-high` | Gemini 3.1 Pro High Thinking |
| `gemini-3-1-pro-low` | Gemini 3.1 Pro Low Thinking |
| `gemini-3.1-pro` | Gemini 3.1 Pro |
| `gemini-3.0-flash` | Gemini 3 Flash (+ High/Low/Medium/Minimal) |
| `gemini-3-pro` | Gemini 3 Pro |

### 4.4 其他

| 模型 | 说明 |
|------|------|
| GPT-OSS 120B Medium Thinking | 开源模型 |
| xAI Grok-3 mini Thinking | xAI 模型 |
| GPT-4.1 / GPT-4o | 旧版模型 |

---

## 5. 通信时序

### 5.1 IDE 启动序列

```
1. PRI * (HTTP/2 preface)
2. GET  /GetCompletions          ← WebSocket 升级
3. POST /GetUserStatus           ← 获取用户信息 + 模型列表
4. POST /Ping                    ← 心跳开始
5. POST /RecordAnalyticsEvent    ← 上报启动事件
```

### 5.2 Cascade 对话流程

```
1. POST /CheckUserMessageRateLimit  ← 检查频率限制 (1.7KB req → 33B resp)
2. POST /GetChatMessage             ← 发送用户消息 (30-94KB req)
   ← Streaming response              多个 gzip protobuf frames
3. POST /RecordCortexTrajectoryStep ← 上报每步操作 (1.8-8.9KB)
4. POST /RecordCortexGeneratorMetadata ← 上报生成元数据 (5-83KB)
```

### 5.3 心跳/定期任务

```
每 ~30 秒:
  POST /Ping                        ← 保活
  POST /GetUserStatus               ← 刷新状态（有时流式返回）
  
每 ~60 秒:
  POST /GetUserJwt                  ← JWT 刷新（如需要）
```

---

## 6. GetChatMessage 请求结构分析

- **请求大小**：30-94 KB（包含完整对话上下文 + 代码 + 文件内容）
- **压缩**：gzip 压缩的 Connect proto
- **关键字段**（从字符串提取）：
  - `bot-{uuid}` — 对话中每条消息的 ID
  - `msg_vrtx_{id}` — Vertex 消息 ID
  - `req_vrtx_{id}` — Vertex 请求 ID
  - `toolu_vrtx_{id}` — 工具调用 ID
  - Conversation UUID（如 `6b36b028-9b86-495a-b9ff-e85b766c48f1`）

### 流式响应结构

- 每帧 100-300 字节（gzip 压缩）
- 解压后包含增量文本 token
- 最终帧为 Trailer（JSON，含 grpc-status）
- 典型一次回复：20-50 个 stream frames，总计 8-26 KB

---

## 7. 错误响应格式

### Connect Error (JSON)

```json
{
  "code": "invalid_argument",
  "message": "an internal error occurred (error ID: 0aa4256de1024cdeb8b4afff5fd23ba2)"
}
```

### 常见错误码

| code | 含义 | 出现场景 |
|------|------|----------|
| `unknown` | 未知错误 | `all API providers are over their global rate limit for trial users` |
| `unauthenticated` | 认证失败 | `primary API key auth not allowed` |
| `invalid_argument` | 参数错误 | 内部错误（generic） |
| `permission_denied` | 权限拒绝 | `API key not found` |

---

## 8. 流量特征

| 指标 | 值 |
|------|------|
| **请求最多的端点** | RecordCortexTrajectoryStep (315次/30min) |
| **最大单请求** | RecordProfilingData (~396 KB) |
| **最频繁** | Ping (~3次/min) + TrajectoryStep (~10次/min) |
| **流式端点** | GetChatMessage (29次), GetUserStatus (41次) |
| **所有 23B 响应** | 空成功 proto frame（通用 ACK） |
| **遥测占比** | ~70% 的请求是遥测/分析上报 |

---

## 9. 安全发现

1. **JWT 明文传输**（TLS 保护内）：包含完整的 team_config、api_key、email
2. **API Key 格式**：`sk-ws-01-{base64}` 可被识别和提取
3. **大量遥测**：IDE 将完整代码上下文、用户操作轨迹、Git 信息上报
4. **GetChatMessage 包含完整上下文**：请求中含完整对话历史 + 打开的文件内容
5. **Team Config 功能开关**：可通过 JWT 中的 team_config 控制 IDE 功能

---

## 10. Protobuf 服务定义（推断）

```protobuf
// 推断的服务定义（基于流量分析）

package exa.api_server_pb;

service ApiServerService {
  rpc GetChatMessage(GetChatMessageRequest) returns (stream GetChatMessageResponse);
  rpc GetCompletions(GetCompletionsRequest) returns (stream GetCompletionsResponse);
  rpc GetCommandModelConfigs(GetCommandModelConfigsRequest) returns (GetCommandModelConfigsResponse);
  rpc GetModelStatuses(GetModelStatusesRequest) returns (GetModelStatusesResponse);
  rpc GetStatus(GetStatusRequest) returns (GetStatusResponse);
  rpc GetDefaultWorkflowTemplates(GetDefaultWorkflowTemplatesRequest) returns (GetDefaultWorkflowTemplatesResponse);
  rpc CheckUserMessageRateLimit(CheckUserMessageRateLimitRequest) returns (CheckUserMessageRateLimitResponse);
  rpc Ping(PingRequest) returns (PingResponse);
  rpc RecordCortexTrajectoryStep(RecordCortexTrajectoryStepRequest) returns (RecordCortexTrajectoryStepResponse);
  rpc RecordCortexGeneratorMetadata(RecordCortexGeneratorMetadataRequest) returns (RecordCortexGeneratorMetadataResponse);
  rpc RecordCortexExecutionMetadata(RecordCortexExecutionMetadataRequest) returns (RecordCortexExecutionMetadataResponse);
  rpc RecordTrajectorySegmentAnalytics(RecordTrajectorySegmentAnalyticsRequest) returns (RecordTrajectorySegmentAnalyticsResponse);
  rpc RecordAsyncTelemetry(RecordAsyncTelemetryRequest) returns (RecordAsyncTelemetryResponse);
  rpc RecordEvent(RecordEventRequest) returns (RecordEventResponse);
  rpc RecordGitTelemetry(RecordGitTelemetryRequest) returns (RecordGitTelemetryResponse);
  rpc RecordProfilingData(RecordProfilingDataRequest) returns (RecordProfilingDataResponse);
}

package exa.seat_management_pb;

service SeatManagementService {
  rpc GetUserStatus(GetUserStatusRequest) returns (stream GetUserStatusResponse);
  rpc GetProfileData(GetProfileDataRequest) returns (GetProfileDataResponse);
}

package exa.auth_pb;

service AuthService {
  rpc GetUserJwt(GetUserJwtRequest) returns (GetUserJwtResponse);
}

package exa.analytics_pb;

service AnalyticsService {
  rpc RecordCortexTrajectoryStep(RecordCortexTrajectoryStepRequest) returns (RecordCortexTrajectoryStepResponse);
}

package exa.product_analytics_pb;

service ProductAnalyticsService {
  rpc RecordAnalyticsEvent(RecordAnalyticsEventRequest) returns (RecordAnalyticsEventResponse);
}
```

---

*生成时间：2026-04-10 | 数据源：MITM 全量抓包 capture_20260410_011032.jsonl*
