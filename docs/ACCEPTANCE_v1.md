# HarmonClaw v1.0 验收报告

## 任务完成清单 (TASK 51-100)

| 范围 | 任务 | 状态 |
|------|------|------|
| v0.8 | TASK 51-60 意图/上下文/管道/路由/记忆/BM25/Engram/LLM/集成/配置 | ✅ |
| v0.9 | TASK 61-65 端云协同 hc-edge/前端/通道/设备/离线 | ✅ |
| v1.0 | TASK 66-70 doc_perceiver/web_search/tts/openclaw/proxy 深度实现 | ✅ |
| v1.1 | TASK 71-75 Governor 安全/审计/IronClaw/HTTPS/CORS | ✅ |
| v1.2 | TASK 76-79 前端对话/Governor/Architect/导航 | ✅ |
| v1.3 | TASK 80-82 安装脚本/签名/首页 | ✅ |
| v1.4 | TASK 83-85 API 文档/SDK/CLI 完善 | ✅ |
| v1.4 | TASK 86 压力测试 | ✅ |
| v1.5 | TASK 87-90 安全/端到端/基准/模糊测试 | ✅ |
| v1.5 | TASK 91-93 内存/启动/请求管道优化 | ✅ |
| v1.6 | TASK 94 CHANGELOG | ✅ |
| v1.6 | TASK 95-98 README/代码清理/.cursorrules/ironclaw_rules | ✅ |
| v1.6 | TASK 99 git push + tag | ✅ v1.6.0 已打 |
| v1.6 | TASK 100 最终验收报告 | ✅ |

## 验证项

- [x] `go test ./...` 全绿
- [x] RISC-V 交叉编译: `CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build ./cmd/harmonclaw/`
- [x] 纯 Go 标准库，零第三方依赖
- [x] 配置文件: configs/{governor,audit,ironclaw_rules,security,tts,openclaw,proxy_claw}.json

## 新增能力

- 意图识别、上下文管理、响应管道
- Viking BM25 检索、Engram 格式
- 技能: doc_perceiver(HTML/CSV)、web_search(SearXNG/缓存)、tts(Edge 代理)、proxy(重试/并发)
- Governor: 防火墙配置、路径 blocklist、可疑头拦截
- IronClaw: 路径级策略矩阵
- TLS、CORS、CSP 安全头
- 前端: 对话空状态、加载动画、Governor 刷新、导航快捷
- API 文档: /api-docs、/landing 静态页
- SDK: SetTimeout、SetToken、ExecuteSkill、Sovereign、LedgerLatest
- CLI (hc): health、skills、chat、ledger、req
- 测试: 安全测试、E2E、基准、模糊
- 优化: sync.Pool 缓冲池、并行启动 SHA、fast-path 请求管道

---

## v2 执行完成（Brave 直连 + Provider 抽象）

### 任务 A 摘要

- 新增 `pkg/bravesearch`：直连 `api.search.brave.com`，请求走调用方传入的 `http.Client`（运行时统一为 `governor.SecureClient()`）。
- 新增 `skills/brave_search.go`（`package skills`）：注册技能 `brave_search`，`BRAVE_API_KEY` / `configs` 中的 `brave_api_key`；可选 `configs/brave_search.json`（`search_lang`、`default_count`）；导出 `BraveSearchNormalizedJSON`、`BraveSearchConfigured` 供 Butler / LLM 工具循环使用。
- `web_search` 技能：路由为 `HC_SEARCH_API` → Brave（有 Key）→ DuckDuckGo；删除 Searx / `HC_SEARCH_SEARXNG` 与 `fetch_inject.go`。
- Butler 注入：优先 Brave 直连，失败再回退 `web_search` 技能。
- `configs/sovereignty.json` 连通白名单增加 `api.search.brave.com`；`main` 移除 Searx 主机动态加白逻辑。

### 任务 B 摘要

- 新增 `providers/`：`provider.go`（含 `ChatStream`）、`deepseek.go`、`router.go`；`llm.NewProvider()` 使用 `Router` + `DeepSeekProvider`（`governor.SecureClient()`）。
- 新增 `llm/router_llm.go`：非流式 `Chat` 在配置了 Brave 时附带 `web_search` 工具定义，解析 `tool_calls` 后调用 `skills.BraveSearchNormalizedJSON` 多轮补全；流式仍走 `Router.ChatStream` → DeepSeek。
- 删除原 `llm/deepseek.go`（直连实现迁至 `providers`）。

### 交叉编译

- `GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build ./...` 已通过（本地验证）。

### 新增 / 修改 / 删除文件（概要）

| 操作 | 路径 |
|------|------|
| 新增 | `pkg/bravesearch/bravesearch.go` |
| 新增 | `skills/brave_search.go` |
| 新增 | `providers/provider.go`, `providers/deepseek.go`, `providers/router.go` |
| 新增 | `llm/router_llm.go` |
| 新增 | `configs/brave_search.json` |
| 修改 | `skills/web_search/search.go`, `skills/web_search/web_search_test.go`, `butler/web_search_inject.go`, `cmd/harmonclaw/main.go`, `configs/config.go`, `configs/sovereignty.json`, `configs/skill-quotas.json`, `configs/policies.json`, `sandbox/sandbox.go`, `.env.example`, `README.md`, `.cursorrules`, `docs/ACCEPTANCE_v1.md` |
| 删除 | `llm/deepseek.go`, `skills/web_search/fetch_inject.go` |

### PR

- 请在本地执行两次 commit（`feat(search): ...` / `feat(llm): ...`）后推送并创建 PR：`feat: brave search direct + provider abstraction`。PR 链接需在 GitHub 上由你创建后填写。

---

> ✅ **v3.0 — Edge TTS 语音输出**
>
> - **依赖库**：`github.com/difyz9/edge-tts-go` **v0.0.3**（纯 Go，经 `gorilla/websocket` 连接微软 Edge TTS；**riscv64 + `CGO_ENABLED=0` 交叉编译通过**）。
> - **出网与主权**：合成前对 Edge 相关主机做 **`AllowOutboundHost`**（与 **`SecureClient`** 同规则）；v4.0.1 起同时校验 **`speech.platform.bing.com`** 与 **`api.msedgeservices.com`**（见文末 v4.0.1 callout）。
> - **新增文件**：`skills/edge_tts.go`，`api/tts_handler.go`，`gateway/edge_tts.go`，`configs/edge_tts.json`。
> - **修改文件**：`governor/client.go`（`AllowOutboundHost` + `RoundTrip` 复用），`cmd/harmonclaw/main.go`，`configs/sovereignty.json`，`configs/ironclaw_rules.json`，`configs/skill-quotas.json`，`configs/policies.json`，`sandbox/sandbox.go`，`.env.example`，`.cursorrules`，`go.mod` / `go.sum`。
> - **HTTP**：`POST /v1/audio/speech`（OpenAI 兼容子集），`Content-Type: audio/mpeg`；技能 **`edge_tts`** 已注册（JSON 返回 base64 MP3）。
> - **本地 MP3 验证**：需在可访问微软 TTS 的网络下执行 `go run ./cmd/harmonclaw` 后，用文档中的 `Invoke-WebRequest` 拉取 `test.mp3`（本 CI 环境未自动跑通外网合成）。
> - **提交**：message `feat(tts): edge tts voice output via WebSocket`；**commit hash** 以 `git log -1 --oneline` 为准。

---

> ✅ **v4.0.1 — Edge TTS 主权白名单（双域名校验）**
>
> - **问题**：仅校验 `api.msedgeservices.com`，与现场 WSS 实际域名 `speech.platform.bing.com` 不一致，导致 `AllowOutboundHost` 误拒或白名单不全时报错。
> - **修改文件**：`skills/edge_tts.go`（`EdgeTTSOutboundHosts` + `allowEdgeTTSOutbound()` 对 **`speech.platform.bing.com`** 与 **`api.msedgeservices.com`** 依次校验）、`configs/sovereignty.json`（白名单顺序调整，两域名均保留）。
> - **edge-tts-go v0.0.3 源码内域名**：`internal/constants` 中 WSS/语音列表为 **`api.msedgeservices.com`**；`VoiceHeaders` 含 **`speech.platform.bing.com`**（Authority）；现场/新版本可能走 bing WSS，故两域名均放行。
> - **交叉编译**：`GOOS=linux GOARCH=riscv64 CGO_ENABLED=0 go build ./...` 已通过（本地）。
> - **本地 MP3**：需在可访问微软服务的网络下用 `Invoke-WebRequest` 自测（本环境未自动跑外网）。
> - **提交**：message `fix(tts): add speech.platform.bing.com to sovereignty whitelist`（以 `git log -1 --oneline` 为准）。
