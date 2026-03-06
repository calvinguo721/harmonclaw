# HarmonClaw — AGENTS.md

## Identity

You are an AI coding agent working on HarmonClaw, a sovereign AI runtime for RISC-V + OpenHarmony devices.

## Architecture

Three-core model: Governor (security/policy), Butler (user interaction), Architect (skill execution).
Cores communicate ONLY through bus/bus.go. Direct cross-core imports are forbidden.

## Key Rules

See .cursorrules for the full 15 IRON RULES. Top 3 to remember:

1. Stdlib only — no third-party Go modules
2. Every file must cross-compile: CGO_ENABLED=0 GOOS=linux GOARCH=riscv64
3. Single file < 300 lines. Define interface first, then implement.

## Running

```
go run ./cmd/harmonclaw/
# Health: GET http://localhost:8080/v1/health
# Chat: POST http://localhost:8080/v1/chat/completions
# Skills: POST http://localhost:8080/v1/skills/execute
```

## Testing

```
go build ./cmd/harmonclaw/
CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o /dev/null ./cmd/harmonclaw/
```
