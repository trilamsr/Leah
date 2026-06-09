#!/usr/bin/env bash
set -euo pipefail

echo "==> go build"
go build ./...

echo "==> go test (unit)"
go test ./...

echo "==> go vet"
go vet ./...

echo "==> golangci-lint (if installed)"
if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
else
  echo "  (skipped — install with: brew install golangci-lint)"
fi

echo "==> all checks passed"
