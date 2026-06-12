# terrawatch

**Catch Terraform drift before it causes an incident.**

terrawatch runs `terraform plan` on your stacks on a schedule, and when real infrastructure no longer matches your code, it automatically opens a pull request — so your team can review and fix it.

No servers. No Kubernetes. Drop it into any existing CI pipeline in minutes.

Works with both **Terraform** and **OpenTofu** — the binary is auto-detected, no configuration needed.

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
- **Per-stack override**: add `ignore` inside a stack block — it is appended to the global list.

When ignore rules hide changes, the PR body and terminal output show a count:
`(3 changes hidden by ignore rules)`.

Glob patterns use `path.Match` semantics: `*` matches any sequence of characters within a segment (dots are literal, not segment separators).

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

## Try it immediately

No config file needed. Just run it in any Terraform directory:

```bash
cd infra/production
terrawatch detect
```

Or scan everything at once:

```bash
terrawatch detect --recursive ./infra
```

Using OpenTofu? It just works — `tofu` is picked up automatically when `terraform` isn't installed, or force it explicitly:

```bash
terrawatch detect --bin tofu
```

Output:

```
  terrawatch v0.1.0

  no config file — local mode (dry-run)

  engine: terraform

  Scanning 3 stack(s)

  ✓  vpc                  no drift
  ⚠  eks                  drift detected  +1 ~0 -0
  ✓  rds                  no drift

  ──────────────────────────────────────────────────
  3 scanned  ·  1 drifted  ·  2 clean
```

Local mode is always a dry-run — it prints results and exits. No PR is opened without a config file.

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
# Global ignore rules — applies to all stacks.
ignore:
  - resource: string       # glob pattern on resource address
    attributes: []         # optional dot-paths; empty = drop the whole resource

stacks:
  - name: string           # display name for this stack
    path: string           # path to the terraform root module
    vars_file: string      # optional .tfvars file
    backend_config:        # optional backend key/value overrides
      key: value
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
```

---

## Why not Atlantis or tf-controller?

| | Atlantis | tf-controller | terrawatch |
|---|---|---|---|
| Requires a running server | Yes | Yes (needs K8s) | No |
| Detects drift automatically | No | No | Yes |
| Opens a PR/MR on drift | Yes (on PR only) | No | Yes |
| GitHub + GitLab | GitHub only | No | Yes |
| OpenTofu support | Partial | No | Yes (auto-detected) |
| Stored cloud credentials | Yes | Yes | No (OIDC) |

terrawatch is not trying to replace Atlantis. It fills the gap: **automatic drift detection with no infrastructure to run**.

---

## License

MIT — see [LICENSE](LICENSE)
