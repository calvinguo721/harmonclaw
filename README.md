# HarmonClaw v1.0

主权 AI 运行时，面向 RISC-V + OpenHarmony 设备。纯 Go 标准库，零第三方依赖，支持 CGO_ENABLED=0 交叉编译。

- **控制台**: http://localhost:8080
- **首页**: http://localhost:8080/landing
- **API 文档**: http://localhost:8080/api-docs

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

## 快速开始

```bash
go build ./cmd/harmonclaw/
go test ./...
go run ./cmd/harmonclaw/
```

访问 http://localhost:8080

## API 端点列表（21 个）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /v1/health | 健康检查，三核状态 + skills |
| GET | /v1/governor/sovereignty | 当前主权模式与白名单 |
| POST | /v1/governor/sovereignty | 切换主权模式（shadow/airlock/opensea） |
| GET | /v1/audit/query | 审计查询（query params） |
| POST | /v1/audit/query | 审计查询（JSON body） |
| GET | /v1/butler/persona | 人格列表与默认 |
| POST | /v1/butler/persona | 切换人格 |
| POST | /v1/chat/completions | 对话（支持 stream:true SSE） |
| POST | /v1/skills/execute | 技能执行 |
| POST | /v1/engram/inject | 记忆注入 |
| GET | /v1/ledger/latest?limit=N | 最新审计记录（默认 20） |
| GET | /v1/ledger/trace?action_id=xxx | 按 action_id 追踪链路 |
| POST | /v1/token | 获取 Bearer Token |
| POST | /v1/auth/login | 登录（username+password，返回 JWT） |
| GET | /v1/architect/skills | 技能注册表 |
| POST | /v1/architect/pipeline/execute | Pipeline 执行 |
| GET | /v1/architect/crons | Cron 任务列表 |
| GET | /v1/viking/snapshots | 快照列表 |
| GET | /v1/viking/search | 全文检索（GET） |
| POST | /v1/viking/search | 全文检索（POST） |
| GET | /debug/vars | expvar 指标（endpoint/skill 统计、延迟） |

## 配置文件

| 文件 | 说明 |
|------|------|
| configs/config.json | 主配置 |
| configs/governor.json | 防火墙（path blocklist、body limit） |
| configs/audit.json | 审计（retention、max_entries） |
| configs/ironclaw_rules.json | 路径级安全矩阵 |
| configs/security.json | CORS、CSP |
| configs/policies.json | IronClaw 策略 |
| configs/llm.json | LLM 路由 |
| configs/tts.json | TTS 配置 |

环境变量：`HC_PORT`、`HC_DATA_DIR`、`HC_TLS_CERT`、`HC_TLS_KEY`、`DEEPSEEK_API_KEY`、`HC_SEARCH_API`、`HC_SEARCH_SEARXNG`、`HC_TTS_ENDPOINT`

## 铁律概要（.cursorrules）

1. **Stdlib only** — 仅 Go 标准库
2. **Cross-compile** — 每次提交必须通过 `CGO_ENABLED=0 GOOS=linux GOARCH=riscv64` 编译
3. **Network Sovereignty** — 出网必须经 `governor.SecureClient()`
4. **Action Trace** — 每条执行链携带 action_id，可追溯
5. **Write Safety** — 原子写入（.tmp → fsync → Rename）
6. **Core Bus** — 三核仅通过 bus 通信，禁止直接 import

完整 15 条见 `.cursorrules`。

## 构建与安装

```bash
make build      # 本地
make build-rv   # RISC-V 交叉编译
make clean      # 清理
./scripts/install.sh sign .  # 从源码构建并安装
./scripts/checksum.sh sign harmonclaw   # 生成 SHA256
./scripts/checksum.sh verify harmonclaw # 校验
```

## CLI (hc)

```bash
go build ./cmd/hc/
hc health
hc skills
hc chat "你好"
hc ledger 10
```

## 许可证

BSL 1.1（Business Source License 1.1）
