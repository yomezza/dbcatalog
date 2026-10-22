#!/usr/bin/env bash
set -euo pipefail
cd /workspace/test
export PATH=/workspace/.tools/go/bin:/workspace/.tools/bin:$PATH
export GOPATH=/workspace/.cache/gopath
export GOBIN=/workspace/.tools/bin
export GOCACHE=/workspace/.cache/go-build
export GOMODCACHE=/workspace/.cache/go-mod
export GOPROXY='https://proxy.golang.org|direct'
export npm_config_cache=/workspace/.cache/npm
if [ ! -x /workspace/.tools/go/bin/go ]; then
  mkdir -p /workspace/.tools
  archive=$(mktemp /tmp/catalogdb-go.XXXXXX.tar.gz)
  trap 'rm -f "$archive"' EXIT
  curl -fsSL https://go.dev/dl/go1.27.3.linux-amd64.tar.gz -o "$archive"
  printf '%s  %s\n' '65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40' "$archive" | sha256sum --check --status
  tar -xzf "$archive" -C /workspace/.tools
fi
go version
go mod download
go install github.com/wailsapp/wails/v2/cmd/wails@v2.17.0
npm --prefix frontend ci
npm --prefix frontend run build
