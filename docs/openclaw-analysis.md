# OpenClaw 源码分析报告

> 用于 HarmonClaw Go 重写规划。基于 OpenClaw 官方文档与仓库结构分析。

## 1. 概述

- **仓库**: [github.com/openclaw/openclaw](https://github.com/openclaw/openclaw)
- **语言**: TypeScript (86.9%)
- **规模**: 约 6.8M tokens，4,885 文件
- **许可**: MIT
- **运行时**: Node ≥22

OpenClaw 是个人 AI 助手框架，支持多通道（WhatsApp、Telegram、Slack、Discord 等）、多端（macOS/iOS/Android）、语音唤醒、Canvas 等。HarmonClaw 的目标是将其核心逻辑用 Go 重写，适配 RISC-V 与边缘设备。

## 2. 核心架构

```
Channels (WhatsApp/Telegram/Slack/...) 
        │
        ▼
┌───────────────────────────────┐
│         Gateway               │
│    (WebSocket 控制平面)         │
│    ws://127.0.0.1:18789       │
└──────────────┬────────────────┘
               │
               ├─ Pi agent (RPC)
               ├─ CLI (openclaw …)
               ├─ WebChat UI
               ├─ macOS app
               └─ iOS / Android nodes
```

### 2.1 目录结构（关键）

| 目录 | 职责 |
|------|------|
| `gateway/` | 核心运行时，WS/HTTP 服务，端口 18789 |
| `agents/` | 默认 agent 模板与示例 |
| `skills/` | 内置能力（浏览器、爬虫等） |
| `cli/` | 命令行 |
| `channels/` | 通道集成（Telegram、Slack、Discord 等） |
| `extensions/` | 34 个插件 |
| `apps/` | macOS、iOS、Android 原生应用 |

### 2.2 协议模型

- **传输**: WebSocket，JSON 文本帧
- **握手**: 首帧必须为 `connect`
- **请求**: `{type:"req", id, method, params}` → `{type:"res", id, ok, payload|error}`
- **事件**: `{type:"event", event, payload, seq?, stateVersion?}`
- **幂等**: 副作用方法（`send`, `agent`）需要 idempotency key

## 3. Agent 循环（核心逻辑）

Agent 循环是「消息 → 动作 → 回复」的权威路径：

1. **入口**: CLI `agent` 命令、Gateway RPC `agent` / `agent.wait`
2. **流程**:
   - `agent` RPC 校验参数，解析 session，持久化元数据，立即返回 `{runId, acceptedAt}`
   - `runEmbeddedPiAgent`: 按 session 串行化运行，解析 model + auth，订阅 pi 事件，流式输出
   - `subscribeEmbeddedPiSession`: 桥接 pi-agent-core 事件 → OpenClaw 流
     - tool 事件 → `stream: "tool"`
     - assistant deltas → `stream: "assistant"`
     - lifecycle → `stream: "lifecycle"` (`phase: start|end|error`)
3. **队列**: 每 session 一条 lane，可选全局 lane，防止 tool/session 竞态
4. **超时**: 默认 600s，`agent.wait` 默认 30s

### 3.1 关键 Hook 点

- `gateway_start` / `gateway_stop`
- `session_start` / `session_end`
- `message_received` / `message_sending` / `message_sent`
- `before_tool_call` / `after_tool_call`
- `before_compaction` / `after_compaction`
- `agent_end`
- `before_prompt_build` / `before_model_resolve`

## 4. Session 模型

- **主 session**: 直接聊天用 `agent::`（默认 `main`），群聊有独立 key
- **Session key 映射**:
  - `main`: `agent::`
  - `per-channel-peer`: `agent:::dm:`
  - `per-account-channel-peer`: `agent::::dm:`
  - 群组: `agent:::group:` 或 `agent:::channel:`
  - Cron: `cron:`
  - Webhook: `hook:`
- **存储**:
  -  transcript: `~/.openclaw/agents/<id>/sessions/<key>.jsonl`
  - store: `~/.openclaw/agents/<id>/sessions/sessions.json`
- **维护**: `pruneAfter`、`maxEntries`、`maxDiskBytes`、`rotateBytes` 等

## 5. 技能（Skills）

- **位置**: `~/.openclaw/workspace/skills/`，每个技能有 `SKILL.md`
- **注入**: `AGENTS.md`、`SOUL.md`、`TOOLS.md` 注入到 system prompt
- **ClawHub**: 技能注册表，可自动搜索并拉取新技能

## 6. HarmonClaw 重写映射

| OpenClaw 概念 | HarmonClaw 对应 |
|---------------|-----------------|
| Gateway WS 控制平面 | gateway/ + bus/ |
| Agent 循环 | engine/conversation_engine.go |
| Session 管理 | butler/conversation.go + viking |
| 技能路由 | engine/skill_router.go |
| 记忆/Compaction | engine/memory_engine.go + butler/memory.go |
| Channel 抽象 | channel/channel.go |
| Pi agent RPC | architect/ + llm/ |

### 6.1 已实现

- openclaw_proxy: HTTP 转发到 OpenClaw Gateway，格式转换（`query`↔`text`，`result` 提取）
- Butler 多轮对话、SSE 流式
- Architect 技能调度、Worker Pool
- Viking 记忆、Ledger、快照

### 6.2 待重写（TASK 37–40）

- **对话引擎**: 从 OpenClaw 提取 context 组装、prompt 构建、流式生命周期
- **技能路由**: 意图匹配、多技能组合
- **记忆引擎**: 短期/长期记忆、检索、衰减、compaction
- **Channel 系统**: HTTP / WebSocket / WeChat 占位接口

## 7. 安全与沙箱

- **main session**: 工具在主机运行，全权限
- **non-main**: 可配置 `sandbox.mode: "non-main"`，每 session Docker 沙箱
- **DM 策略**: `dmPolicy="pairing"` 需配对批准；`dmPolicy="open"` 需显式 allowlist

## 8. 参考链接

- [Gateway 架构](https://docs.openclaw.ai/concepts/architecture)
- [Agent 循环](https://docs.openclaw.ai/concepts/agent-loop)
- [Session 管理](https://docs.openclaw.ai/concepts/session)
- [Gateway 协议](https://docs.openclaw.ai/gateway/protocol)
- [配置参考](https://docs.openclaw.ai/gateway/configuration)
