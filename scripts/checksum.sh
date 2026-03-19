#!/bin/bash
# HarmonClaw 二进制签名与校验 (SHA-256)
# Usage: ./checksum.sh sign <binary>   - 生成 .sha256
#        ./checksum.sh verify <binary>  - 校验
set -e
cmd="${1:-sign}"
bin="${2:-harmonclaw}"

sha_cmd() {
  if command -v sha256sum &>/dev/null; then
    sha256sum "$1"
  elif command -v shasum &>/dev/null; then
    shasum -a 256 "$1"
  else
    echo "Error: sha256sum or shasum required"
    exit 1
  fi
}

verify_cmd() {
  if command -v sha256sum &>/dev/null; then
    sha256sum -c "$1"
  elif command -v shasum &>/dev/null; then
    shasum -a 256 -c "$1"
  else
    echo "Error: sha256sum or shasum required"
    exit 1
  fi
}

case "$cmd" in
  sign)
    if [ ! -f "$bin" ]; then
      echo "Usage: $0 sign <binary_path>"
      echo "  Generates <binary>.sha256"
      exit 1
    fi
    sha_cmd "$bin" | tee "${bin}.sha256"
    echo "Signed: ${bin}.sha256"
    ;;
  verify)
    if [ ! -f "$bin.sha256" ]; then
      echo "Usage: $0 verify <binary_path>"
      echo "  Expects <binary>.sha256"
      exit 1
    fi
    verify_cmd "${bin}.sha256"
    echo "OK: $bin"
    ;;
  *)
    echo "Usage: $0 {sign|verify} <binary_path>"
    exit 1
    ;;
esac
