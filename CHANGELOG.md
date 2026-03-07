# Changelog

## [v1.0] - 2026-03

### Added
- Intent recognition engine (configs/intents.json)
- Session context manager with sliding window + Viking memory injection
- Response pipeline: intent → context → LLM/skill → post-process → memory
- Skill router: TF-IDF matching, chaining, fallback
- Memory engine: short/long-term, extraction, decay, Jaccard merge
- Viking BM25 hybrid search, Chinese segmentation, Porter stemmer
- Viking Engram format (data/viking/engrams/)
- LLM router: DeepSeek/OpenAI/Ollama, sovereignty-aware
- doc_perceiver: HTML/CSV, keyword extraction, 1MB limit
- web_search: SearXNG, cache, concurrency limit
- tts: config, cache, Edge TTS proxy mode
- openclaw_proxy: retry backoff, concurrency, shadow
- mimicclaw/nanoclaw/picoclaw: config, retry, shadow
- Governor firewall: path blocklist, suspicious headers, config
- Audit: severity, retention, max_entries
- IronClaw path rules (configs/ironclaw_rules.json)
- HTTPS/TLS (HC_TLS_CERT, HC_TLS_KEY)
- CORS/CSP (configs/security.json)
- Chat: empty state, loading indicator
- Governor panel: refresh button
- Nav: smooth scroll, Viking tab shortcut
- Install script (scripts/install.sh)
- Checksum script (scripts/checksum.sh)
- Landing page (web/landing.html)

### Changed
- Firewall uses FirewallConfig from configs/governor.json
- LedgerEntry: optional Severity, UserID, ExtraDetails
- Gateway: ListenAndServeTLS for TLS support

## [v0.7] - 2026-03

### Added
- Event bus with Publish/Subscribe (sovereignty.changed, config.reloaded, skill.degraded, etc.)
- Health dashboard (web/health.html) with 5s auto-refresh
- Go SDK client (sdk/client.go)
- hc CLI (cmd/hc): health, skills, version, sovereign status, audit query
- RV2 deployment script + systemd service
- BSL 1.1 license, Apache 2.0 community prep, OPEN-CORE.md
- Unified config loader (config.Get(), Validate(), priority chain)
- OpenClaw source analysis (docs/openclaw-analysis.md)
- Conversation engine, skill router, memory engine (engine/)
- Channel system (HTTP/WebSocket/WeChat placeholders)
- Auto-updater with rollback (updater/)
- Opt-in telemetry (HC_TELEMETRY=off)
- Dynamic rate limit API (PUT /v1/governor/ratelimit)
- Backup/restore (backup/)
- Viking panel in web dashboard
- Integration test suite (tests/)

### Changed
- Config watcher uses bus.Publish for config.reloaded
- Structured request logging with rotation
- Unified error pages with content negotiation
- /v1/version endpoint with ldflags
- Unified nav + skeleton loading in web UI

## [v0.2] - 2026-03

### Added
- Governor, Butler, Architect, Viking cores
- JWT auth, login page, sovereignty whitelist
- doc_perceiver, web_search, tts, proxy skills (real implementations)
- Config hot-reload, graceful shutdown
- expvar metrics, skill quotas, cross-compile scripts
