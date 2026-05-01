#!/usr/bin/env sh

set -eu

echo "Running pre-commit checks..."
go vet ./...

if command -v golangci-lint >/dev/null 2>&1; then
  golangci-lint run ./...
else
  echo "golangci-lint is not installed. Install from https://golangci-lint.run/welcome/install/"
  exit 1
fi

go test ./...
echo "Pre-commit checks passed."
