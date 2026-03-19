# Build HarmonClaw for RISC-V 64 (e.g. Orange Pi RV2)
$env:GOOS = "linux"
$env:GOARCH = "riscv64"
$env:CGO_ENABLED = "0"
go build -o harmonclaw-riscv64 ./cmd/harmonclaw/
Write-Host "Built: harmonclaw-riscv64"
