# Changelog

## [v0.7] - 2026-03

### Added
- Event bus with Publish/Subscribe (sovereignty.changed, config.reloaded, skill.degraded, etc.)
- Health dashboard (web/health.html) with 5s auto-refresh
- Go SDK client (sdk/client.go)
- hc CLI (cmd/hc): health, skills, version, sovereign status, audit query
- RV2 deployment script + systemd service
- BSL 1.1 license, Apache 2.0 community prep, OPEN-CORE.md

### Changed
- Config watcher uses bus.Publish for config.reloaded
- Structured request logging with rotation
- Unified error pages with content negotiation
- /v1/version endpoint with ldflags

## [v0.2] - 2026-03

### Added
- Governor, Butler, Architect, Viking cores
- JWT auth, login page, sovereignty whitelist
- doc_perceiver, web_search, tts, proxy skills (real implementations)
- Config hot-reload, graceful shutdown
- expvar metrics, skill quotas, cross-compile scripts
