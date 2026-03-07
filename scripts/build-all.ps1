# Build HarmonClaw for all target platforms
$ErrorActionPreference = "Stop"
$buildTime = (Get-Date -Format "2006-01-02T15:04:05Z07:00")
$gitCommit = (git rev-parse --short HEAD 2>$null) -replace "`n",""
if (-not $gitCommit) { $gitCommit = "unknown" }
$ldflags = "-X harmonclaw/gateway.buildTime=$buildTime -X harmonclaw/gateway.gitCommit=$gitCommit"

# Windows amd64
$env:GOOS = ""
$env:GOARCH = ""
$env:CGO_ENABLED = "0"
go build -ldflags $ldflags -o harmonclaw-windows-amd64.exe ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-windows-amd64.exe"

# Linux amd64
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -ldflags $ldflags -o harmonclaw-linux-amd64 ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-linux-amd64"

# Linux riscv64
$env:GOOS = "linux"
$env:GOARCH = "riscv64"
go build -ldflags $ldflags -o harmonclaw-riscv64 ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-riscv64"

# Reset
$env:GOOS = ""
$env:GOARCH = ""
Write-Host "Done. All platforms built."
