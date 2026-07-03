# terrawatch

**Catch Terraform drift before it causes an incident.**

[![CI](https://github.com/MaripeddiSupraj/terrawatch/actions/workflows/ci.yml/badge.svg)](https://github.com/MaripeddiSupraj/terrawatch/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/MaripeddiSupraj/terrawatch)](https://github.com/MaripeddiSupraj/terrawatch/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Terraform | OpenTofu](https://img.shields.io/badge/Terraform-%7C%20OpenTofu-7B42BC)](https://opentofu.org)

terrawatch runs `terraform plan` on your stacks on a schedule, and when real infrastructure no longer matches your code, it automatically opens a pull request — so your team can review and fix it.

A free, no-server alternative to driftctl (deprecated) and Terraform Cloud's paid drift detection. No servers, no Kubernetes, no stored cloud credentials. Works with **Terraform** and **OpenTofu** (auto-detected). Drop it into any CI pipeline in minutes.

---

## Try it in 30 seconds

```bash
# Install (macOS / Linux)
brew tap MaripeddiSupraj/terrawatch
brew install terrawatch

# Run it in any Terraform directory — no config, no cloud credentials
cd your-terraform-dir
terrawatch detect
```

```text
  terrawatch 0.3.0

  Scanning 1 stack(s)

  ⚠  production          drift detected  +0 ~1 -0

  1 scanned  ·  1 drifted  ·  0 clean
```

That's local mode — it just prints drift and exits. To make it open PRs automatically, add a [config file](#set-up-automated-pr-creation). Other ways to install are [below](#install).

---

## The problem it solves

Your Terraform code says one thing. Your cloud says another.

This happens constantly — someone clicks in the console, a resource auto-scales, a tag gets added manually. Without drift detection, you won't notice until a `terraform apply` surprises you in production.

terrawatch runs in the background, checks continuously, and brings the diff to your PR queue where your team already works.

---

## How it works

```
Every few hours (scheduled CI job)
  └── terraform plan on each stack
        ├── No changes → silent, nothing happens
        └── Changes found → opens a PR with the full plan diff
```

The PR looks like this:

```
[terrawatch] Drift detected in stack: production

Stack:     production
Path:      ./environments/prod
Detected:  Sun, 27 Apr 2026 06:00:00 UTC

Summary
| Add | Change | Destroy |
|  0  |   1    |    0    |

# aws_instance.web will be updated in-place
~ instance_type = "t3.small" → "t3.medium"
```

If an open drift PR already exists for a stack, terrawatch skips it — no duplicate PRs.

---

## Features

| | What it does |
|---|---|
| [Ignore rules](#ignore-rules) | Filter out known false-drift (tags, autoscaling, managed metadata) so you only see real changes |
| [Real drift vs. unapplied code](#real-drift-vs-unapplied-changes) | `--classify` tells genuine infra drift apart from merged-but-unapplied code |
| [Auto-close resolved PRs](#auto-closing-resolved-drift) | Closes the drift PR automatically when a stack comes back clean |
| [JSON output](#machine-readable-output) | `--format json` for piping into CI / dashboards |
| [Preflight check](#validate-before-you-run) | `terrawatch validate` verifies config, paths, binary, and token before a real run |
| [Parallel scanning](#parallel-scanning) | `--parallel N` plans several stacks at once — big repos scan in a fraction of the time |
| OpenTofu support | `terraform` and `tofu` both work — auto-detected, or force with `--bin` |

### Ignore rules

Teams abandon drift tools because of noise — tags added by cost tools, autoscaling `desired_capacity`, managed resource metadata. Set ignore rules to filter out known false-drift:

```yaml
ignore:
  - resource: "aws_autoscaling_group.*"
    attributes: [desired_capacity]
  - resource: "*"
    attributes: [tags.LastScanned, tags_all]
  - resource: "null_resource.ephemeral_*"
```

- **Without `attributes`**: the entire resource change is dropped.
- **With `attributes`**: only those specific dot-paths are compared. If removing them makes `before == after`, the resource is reported clean.
- **All matching rules combine**: a resource matched by several rules ignores the union of their attributes; a whole-resource rule always wins.
- **Per-stack override**: add `ignore` inside a stack block — it is appended to the global list.

When ignore rules hide changes, the PR body and terminal output show a count:
`(3 changes hidden by ignore rules)`.

Note: ignore rules decide whether a stack counts as drifted — the raw plan diff
embedded in the PR is unmodified terraform output and still shows ignored attributes.

Glob patterns use `path.Match` semantics: `*` matches any sequence of characters within a segment (dots are literal, not segment separators).

### Real drift vs. unapplied changes

A plain `terraform plan` shows changes for two very different reasons:

1. **Real infrastructure drift** — someone changed the cloud outside Terraform.
2. **Unapplied code changes** — code was merged but never `apply`-ed; the cloud still matches state.

Most teams only want to be paged for (1). Run with `--classify` (or set `drift_mode: strict`) and terrawatch runs an extra `plan -refresh-only` to tell them apart:

```bash
terrawatch detect --classify
```

```text
  ⚠  eks                  infra drift        +0 ~1 -0
  ℹ  vpc                  unapplied changes  +1 ~0 -0
```

In **strict** mode, unapplied changes are reported for visibility but do **not** open a PR or fail the run (exit code stays `0`). Only real infra drift triggers a PR and exit code `2`. The trade-off: classification runs a second plan per stack, so scans take longer.

### Machine-readable output

```bash
terrawatch detect --format json
```

Emits a stable JSON document to **stdout** (all human output goes to stderr, so the stream stays pipeable):

```json
{
  "version": "0.3.0",
  "engine": "terraform",
  "scanned": 2, "drifted": 1, "clean": 1, "errors": 0,
  "stacks": [
    { "name": "eks", "path": "./eks", "status": "drift",
      "kind": "infra_drift", "summary": { "add": 0, "change": 1, "destroy": 0 },
      "pr_url": "https://github.com/org/repo/pull/42" },
    { "name": "vpc", "path": "./vpc", "status": "clean" }
  ]
}
```

Use `--quiet` to silence the human output entirely (errors still print to stderr).

### Validate before you run

`terrawatch validate` is a preflight check — config parses, every stack path exists and has `.tf` files, the terraform/tofu binary resolves, and the GitHub/GitLab token actually authenticates against the repo. Exits `1` on any failure. Wire it into CI ahead of the detect job to fail fast on a bad token or moved directory:

```bash
terrawatch validate --config terrawatch.yaml
```

### Parallel scanning

By default stacks are planned one at a time. With many stacks (or `--classify`, which doubles the plans), pass `--parallel N` or set `concurrency` in the config to run up to N plans concurrently:

```bash
terrawatch detect --parallel 4
```

Results are still printed in stack order, and exit codes are unchanged. Each stack is an independent working directory, so plans don't contend — but they do share provider API rate limits, so start with 4 and raise it if your cloud provider is happy.

### Auto-closing resolved drift

When a stack that previously had an open drift PR comes back clean, terrawatch comments "✅ Drift resolved" and closes the PR, deleting its branch — so your PR queue never fills with stale drift reports. On by default; set `auto_close: false` to disable.

---

## Install

**Homebrew (Mac / Linux):**

```bash
brew tap MaripeddiSupraj/terrawatch
brew install terrawatch
```

**curl (Linux / Mac):**

```bash
# Linux (amd64)
curl -sSL https://github.com/MaripeddiSupraj/terrawatch/releases/latest/download/terrawatch_linux_amd64.tar.gz | tar xz
sudo mv terrawatch /usr/local/bin/

# Mac (Apple Silicon)
curl -sSL https://github.com/MaripeddiSupraj/terrawatch/releases/latest/download/terrawatch_darwin_arm64.tar.gz | tar xz
sudo mv terrawatch /usr/local/bin/
```

**Go:**

```bash
go install github.com/MaripeddiSupraj/terrawatch@latest
```

---

## Local mode

Run `terrawatch detect` in any Terraform directory with no config file — it prints drift and exits (always a dry-run, never opens a PR). Handy for a quick check or a pre-commit hook.

```bash
terrawatch detect --recursive ./infra    # scan every stack under a directory
terrawatch detect --bin tofu             # force OpenTofu (otherwise auto-detected)
```

```text
  terrawatch 0.3.0

  no config file — local mode (dry-run)
  engine: terraform

  Scanning 3 stack(s)

  ✓  vpc                  no drift
  ⚠  eks                  drift detected  +1 ~0 -0
  ✓  rds                  no drift

  3 scanned  ·  1 drifted  ·  2 clean
```

---

## Set up automated PR creation

**1. Create `terrawatch.yaml` in your repo root:**

```yaml
stacks:
  - name: production
    path: ./environments/prod
    vars_file: prod.tfvars     # optional
  - name: staging
    path: ./environments/staging

github:
  repo: your-org/your-infra-repo
  base_branch: main
  labels: [drift, infra]
```

For GitLab:

```yaml
stacks:
  - name: production
    path: ./environments/prod

gitlab:
  repo: your-group/your-project
  base_branch: main
  labels: [drift]
```

**2. Run:**

```bash
# see drift without opening a PR
GITHUB_TOKEN=xxx terrawatch detect --dry-run

# full run — opens a PR for each drifted stack
GITHUB_TOKEN=xxx terrawatch detect
```

---

## Add to an existing pipeline

### GitHub Actions — scheduled drift detection

Drop this into your infra repo. It runs every 6 hours and can be triggered manually.

```yaml
# .github/workflows/drift-detect.yml
name: Drift Detection
on:
  schedule:
    - cron: "0 */6 * * *"
  workflow_dispatch:
    inputs:
      dry_run:
        type: boolean
        default: false

jobs:
  detect:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Configure AWS credentials
        uses: aws-actions/configure-aws-credentials@v4
        with:
          role-to-assume: ${{ secrets.AWS_ROLE_ARN }}
          aws-region: ${{ vars.AWS_REGION }}

      - name: Install terrawatch
        run: |
          curl -sSL https://github.com/MaripeddiSupraj/terrawatch/releases/latest/download/terrawatch_linux_amd64.tar.gz | tar xz
          sudo mv terrawatch /usr/local/bin/

      - name: Detect drift
        run: terrawatch detect --config terrawatch.yaml
        env:
          GITHUB_TOKEN: ${{ secrets.TERRAWATCH_PAT }}
```

Required secrets:

| Secret | What it is |
|---|---|
| `TERRAWATCH_PAT` | GitHub PAT with `repo` scope — for opening PRs |
| `AWS_ROLE_ARN` | IAM role ARN for OIDC auth (no stored keys needed) |

> **Tip:** Use a dedicated PAT instead of the built-in `GITHUB_TOKEN`. The built-in token requires a blanket repo setting to create PRs — a PAT keeps permissions explicit.

### Add as a post-apply check

Run after `terraform apply` to confirm the apply fully converged:

```yaml
- name: Apply
  run: terraform apply -auto-approve tfplan

- name: Verify convergence
  run: terrawatch detect --dry-run --config terrawatch.yaml
  # exits 2 if drift still present → fails the pipeline
```

### GitLab CI

```yaml
drift-detect:
  stage: monitor
  only:
    - schedules
  script:
    - terrawatch detect --config terrawatch.yaml
  variables:
    GITLAB_TOKEN: $MY_GITLAB_PAT
```

---

## CLI reference

```
terrawatch detect [dir...]        check current dir or specified paths
terrawatch detect --recursive     walk all subdirs for terraform stacks
terrawatch detect --dry-run       print drift, do not open a PR
terrawatch detect --config        use a config file (enables PR creation)
terrawatch detect --bin tofu      force a specific terraform/tofu binary
terrawatch detect --classify      tell real infra drift from unapplied code
terrawatch detect --stack NAME    scan only the named stack(s); repeatable
terrawatch detect --parallel N    plan up to N stacks concurrently (default 1)
terrawatch detect --format json   machine-readable output (stdout = pure JSON)
terrawatch detect --quiet         suppress everything except errors
terrawatch validate               preflight: config, paths, binary, token auth
terrawatch version                print version info
```

**Exit codes** (mirrors `terraform plan -detailed-exitcode`):

| Code | Meaning |
|---|---|
| `0` | No drift detected |
| `1` | Error (init/plan failed, PR creation failed) |
| `2` | Drift detected |

So in CI you can tell a broken pipeline apart from real drift:

```bash
terrawatch detect
case $? in
  0) echo "all clean" ;;
  2) pagerduty-alert "infrastructure drift" ;;
  *) pagerduty-alert "drift detection is broken" ;;
esac
```

---

## Configuration reference

```yaml
# How to treat plan changes:
#   all    - any change is drift (default)
#   strict - run a refresh-only plan first; only real infra drift opens a
#            PR / fails the run, unapplied code changes are informational
drift_mode: all

# Close a stack's open drift PR when it comes back clean (default: true)
auto_close: true

# How many stacks to plan concurrently (default: 1, i.e. sequential).
# The --parallel flag overrides this.
concurrency: 1

# Global ignore rules — applies to all stacks.
ignore:
  - resource: string       # glob pattern on resource address
    attributes: []         # optional dot-paths; empty = drop the whole resource

stacks:
  - name: string           # display name for this stack
    path: string           # path to the terraform root module
    vars_file: string      # optional .tfvars file
    backend_config:        # optional key/value pairs passed to init as
      key: value           # -backend-config=key=value (e.g. per-stack state key)
    ignore:                # optional per-stack overrides, appended to global
      - resource: string
        attributes: []

# Use either github OR gitlab — not both

github:
  token: string            # or set GITHUB_TOKEN env var
  repo: owner/repo         # required
  base_branch: main        # default: main
  labels: []               # PR labels
  assignees: []            # GitHub usernames

gitlab:
  token: string            # or set GITLAB_TOKEN env var
  repo: group/project      # required
  url: https://gitlab.com  # for self-hosted GitLab
  base_branch: main        # default: main
  labels: []               # MR labels
  assignees: []            # GitLab usernames

terraform:
  bin_path: tofu           # terraform/tofu binary — auto-detected if omitted
  timeout: 30m             # per-command timeout; "0" disables (default 30m)
```

---

## Why not Atlantis or tf-controller?

| | Atlantis | tf-controller | terrawatch |
|---|---|---|---|
| Requires a running server | Yes | Yes (needs K8s) | No |
| Detects drift automatically | No | No | Yes |
| Real drift vs. unapplied code | No | No | Yes (`--classify`) |
| Opens a PR/MR on drift | Yes (on PR only) | No | Yes |
| Auto-closes resolved drift PRs | No | No | Yes |
| GitHub + GitLab | GitHub only | No | Yes |
| OpenTofu support | Partial | No | Yes (auto-detected) |
| Stored cloud credentials | Yes | Yes | No (OIDC) |

terrawatch is not trying to replace Atlantis. It fills the gap: **automatic drift detection with no infrastructure to run**.

---

## License

MIT — see [LICENSE](LICENSE)
