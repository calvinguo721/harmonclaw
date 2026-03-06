# HarmonClaw

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
    │ 安全/策略   │ │ 对话/交互   │ │ 技能执行    │
    └──────┬──────┘ └──────┬──────┘ └──────┬──────┘
           └───────────────┼───────────────┘
                           │
                    ┌──────▼──────┐
                    │  bus/bus.go │  三核仅通过总线通信
                    └─────────────┘
```

- **Governor**：配额、IronClaw 策略、Token 签发
- **Butler**：Chat 对话、Viking 记忆、RealtimeQueue
- **Architect**：WorkerPool、技能白名单、H-Skill 执行

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
| POST | /v1/chat/completions | 对话（DeepSeek） |
| POST | /v1/skills/execute | 技能执行 |
| POST | /v1/engram/inject | 记忆注入 |
| GET | /v1/ledger/latest | 最新审计记录 |
| GET | /v1/ledger/trace?action_id=xxx | 按 action_id 追踪链路 |
| GET | /debug/vars | expvar 指标 |

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
```
