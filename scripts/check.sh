#!/bin/sh
set -eu
cd "$(git rev-parse --show-toplevel)"
python3 scripts/test-sync-upstream.py
(cd src/web/frontend && npm ci && npm run build)
go test -race -timeout 2m ./...
build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT HUP INT TERM
go build -o "$build_dir/explo" ./src/main/
