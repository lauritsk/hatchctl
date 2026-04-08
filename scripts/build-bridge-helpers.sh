#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
assets_dir=${BRIDGE_HELPERS_DIR:-internal/bridge/generated/assets}
assets_path=$repo_root/$assets_dir

mkdir -p "$assets_path/linux_amd64" "$assets_path/linux_arm64"

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$assets_path/linux_amd64/hatchctl" ./cmd/hatchctl
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o "$assets_path/linux_arm64/hatchctl" ./cmd/hatchctl

chmod 0755 "$assets_path/linux_amd64/hatchctl" "$assets_path/linux_arm64/hatchctl"
