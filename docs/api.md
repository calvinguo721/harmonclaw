# HarmonClaw API 文档

> 自动从 gateway 路由生成

## 健康与版本

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/health | 健康检查，三核状态 |
| GET | /v1/version | 版本信息 |

## Governor

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/governor/sovereignty | 当前主权模式 |
| POST | /v1/governor/sovereignty | 切换模式 (shadow/airlock/opensea) |
| GET | /v1/governor/ratelimit | 限流配置 |
| PUT | /v1/governor/ratelimit | 更新限流 |

## Butler

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /v1/chat/completions | 对话 (stream:true 支持 SSE) |
| GET | /v1/butler/persona | 人格列表 |
| POST | /v1/butler/persona | 切换人格 |

## Architect

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /v1/skills/execute | 技能执行 |
| GET | /v1/architect/skills | 技能注册表 |
| POST | /v1/architect/pipeline/execute | Pipeline 执行 |
| GET | /v1/architect/crons | Cron 列表 |

## Viking

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | /v1/viking/search | 全文检索 |
| GET | /v1/viking/snapshots | 快照列表 |
| POST | /v1/engram/inject | 记忆注入 |

## 审计与认证

| 方法 | 路径 | 说明 |
|------|------|------|
| GET/POST | /v1/audit/query | 审计查询 |
| GET | /v1/ledger/latest | 最新审计 |
| GET | /v1/ledger/trace | 按 action_id 追踪 |
| POST | /v1/token | 获取 Bearer Token |
| POST | /v1/auth/login | 登录 |

## Edge

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | /v1/edge/register | 设备注册 |
| POST | /v1/edge/heartbeat | 心跳 |
| GET | /v1/edge/devices | 设备列表 |
| POST | /v1/edge/command | 下发命令 |

## 调试

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /debug/vars | expvar 指标 |
