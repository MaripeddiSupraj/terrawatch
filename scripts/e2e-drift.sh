#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

for cmd in go terraform; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "missing required command: $cmd" >&2
    exit 1
  fi
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cp "$ROOT/testdata/e2e-drift/main.tf" "$tmp/main.tf"
cat > "$tmp/terraform.tfvars" <<'EOF'
content = "desired-v1"
EOF

terraform -chdir="$tmp" init -input=false >/dev/null
terraform -chdir="$tmp" apply -input=false -auto-approve >/dev/null

(
  cd "$ROOT"
  go build -o "$tmp/terrawatch" .
)

cat > "$tmp/terrawatch.yaml" <<EOF
drift_mode: strict
stacks:
  - name: local-e2e
    path: $tmp
    vars_file: terraform.tfvars
github:
  token: dry-run-only
  repo: example/example
  base_branch: main
EOF

run_detect() {
  local expected_code="$1"
  local expected_text="$2"

  set +e
  local output
  output="$("$tmp/terrawatch" detect --config "$tmp/terrawatch.yaml" --dry-run 2>&1)"
  local code=$?
  set -e

  echo "$output"

  if [[ "$code" -ne "$expected_code" ]]; then
    echo "expected exit code $expected_code, got $code" >&2
    exit 1
  fi
  if ! grep -qi "$expected_text" <<<"$output"; then
    echo "expected output to contain: $expected_text" >&2
    exit 1
  fi
}

echo "==> Case 1: clean infrastructure"
run_detect 0 "no drift"

echo "==> Case 2: unapplied Terraform code"
cat > "$tmp/terraform.tfvars" <<'EOF'
content = "desired-v2"
EOF
run_detect 0 "unapplied"

echo "==> Case 3: real out-of-band drift"
cat > "$tmp/terraform.tfvars" <<'EOF'
content = "desired-v1"
EOF
rm -f "$tmp/managed.txt"
run_detect 2 "infra drift"

echo "PASS: clean, unapplied-code, and real-drift classification all behaved as expected"
