#!/bin/bash
# Deploy HarmonClaw to Orange Pi RV2 via SSH
# Usage: ./deploy-rv2.sh <user@host> [binary_path]

set -e
HOST="${1:?Usage: $0 user@host [binary_path]}"
BIN="${2:-harmonclaw-linux-riscv64}"

echo "Building for RISC-V..."
CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -o "$BIN" ./cmd/harmonclaw/

echo "Uploading to $HOST..."
scp "$BIN" "$HOST:/tmp/harmonclaw"
scp deploy/harmonclaw.service "$HOST:/tmp/"

echo "Installing..."
ssh "$HOST" "sudo mv /tmp/harmonclaw /usr/local/bin/ && sudo chmod +x /usr/local/bin/harmonclaw && sudo mv /tmp/harmonclaw.service /etc/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable harmonclaw && sudo systemctl restart harmonclaw"

echo "Done. Check: ssh $HOST 'systemctl status harmonclaw'"
