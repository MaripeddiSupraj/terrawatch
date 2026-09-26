#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

echo "==> Checking formatting"
unformatted="$(gofmt -l .)"
if [[ -n "$unformatted" ]]; then
  echo "These files are not gofmt-clean:"
  echo "$unformatted"
  exit 1
fi

echo "==> Running go vet"
go vet ./...

echo "==> Running tests with race detector"
go test -race ./...

echo "==> Building terrawatch"
tmp_bin="$(mktemp -t terrawatch.XXXXXX)"
trap 'rm -f "$tmp_bin"' EXIT
go build -o "$tmp_bin" .

echo "==> Smoke-testing version command"
"$tmp_bin" version

echo "PASS: terrawatch verification complete"
