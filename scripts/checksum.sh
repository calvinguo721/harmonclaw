#!/bin/bash
# Generate and verify SHA-256 checksums for HarmonClaw binaries
set -e
BIN="${1:-harmonclaw}"
if [ ! -f "$BIN" ]; then
  echo "Usage: $0 <binary_path>"
  echo "Generates $BIN.sha256"
  exit 1
fi
if command -v sha256sum &>/dev/null; then
  sha256sum "$BIN" | tee "${BIN}.sha256"
elif command -v shasum &>/dev/null; then
  shasum -a 256 "$BIN" | tee "${BIN}.sha256"
else
  echo "No sha256sum/shasum found"
  exit 1
fi
echo "Verify with: sha256sum -c ${BIN}.sha256"
