# HarmonClaw v0.2

主权 AI 运行时，面向 RISC-V + OpenHarmony 设备。纯 Go 标准库，零第三方依赖，支持 CGO_ENABLED=0 交叉编译。

## 架构

三核模型（Life-Centric）：

```
                    ┌─────────────┐
                    │   Gateway   │  HTTP 入口
                    └──────┬──────┘
           ┌───────────────┼───────────────┐
           ▼               ▼               ▼
    ┌─────────────┐ ┌─────────────┐ ┌─────────────┐
    │  Governor   │ │   Butler    │ │  Architect  │
    │ sovereignty │ │ conversation│ │ scheduler   │
    │ audit       │ │ sse         │ │ registry    │
    │ ratelimit   │ │ tts_stream  │ │ pipeline    │
    └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
           └───────────────┼───────────────┘
                           │
                    ┌──────▼──────┐
                    │  bus/bus.go │  三核仅通过总线通信
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Viking    │  store / search / snapshot
                    └─────────────┘
```

- **Governor**：sovereignty 三档拨杆、audit 审计引擎、crypto 加密管道、ratelimit 令牌桶
- **Butler**：conversation 多轮对话、sse 流式引擎、tts_stream 音频分块
- **Architect**：scheduler 优先级调度、registry 技能注册、pipeline 技能管道
- **Viking**：store 键值存储、search 全文检索、snapshot 版本快照

## 快速启动

```bash
make run
# 或
go run ./cmd/harmonclaw/
```

访问 http://localhost:8080

## API 列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/health | 健康检查，三核状态 + skills |
| GET | /v1/governor/sovereignty | 当前主权模式与白名单 |
| POST | /v1/governor/sovereignty | 切换主权模式（shadow/airlock/opensea） |
| POST | /v1/chat/completions | 对话（支持 stream:true SSE） |
| POST | /v1/skills/execute | 技能执行 |
| POST | /v1/engram/inject | 记忆注入 |
| GET | /v1/ledger/latest?limit=N | 最新审计记录（默认 20） |
| GET | /v1/ledger/trace?action_id=xxx | 按 action_id 追踪链路 |
| POST | /v1/token | 获取 Bearer Token |
| GET | /debug/vars | expvar 指标 |

## 配置

环境变量：`HC_PORT`、`HC_DATA_DIR`、`HC_LOG_LEVEL`、`DEEPSEEK_API_KEY`、`HC_AUTH_ENABLED`、`HC_IRONCLAW_POLICIES`、`HC_SOVEREIGNTY_MODE`、`HC_CONFIG`

## 铁律概要（.cursorrules）

1. **Stdlib only** — 仅 Go 标准库
2. **Cross-compile** — 每次提交必须通过 `CGO_ENABLED=0 GOOS=linux GOARCH=riscv64` 编译
3. **Network Sovereignty** — 出网必须经 `governor.SecureClient()`
4. **Action Trace** — 每条执行链携带 action_id，可追溯
5. **Write Safety** — 原子写入（.tmp → fsync → Rename）
6. **Core Bus** — 三核仅通过 bus 通信，禁止直接 import

完整 15 条见 `.cursorrules`。

## 构建

```bash
make build      # 本地
make build-rv   # RISC-V
make test       # vet + build
make smoke      # 端到端 smoke 测试
```
