# Build HarmonClaw for all target platforms
$ErrorActionPreference = "Stop"

# Windows amd64
$env:GOOS = ""
$env:GOARCH = ""
$env:CGO_ENABLED = "0"
go build -o harmonclaw-windows-amd64.exe ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-windows-amd64.exe"

# Linux amd64
$env:GOOS = "linux"
$env:GOARCH = "amd64"
go build -o harmonclaw-linux-amd64 ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-linux-amd64"

# Linux riscv64
$env:GOOS = "linux"
$env:GOARCH = "riscv64"
go build -o harmonclaw-riscv64 ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-riscv64"

# Reset
$env:GOOS = ""
$env:GOARCH = ""
Write-Host "Done. All platforms built."
