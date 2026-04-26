# terrawatch

Detect Terraform infrastructure drift and automatically open a pull request to fix it.

No servers. No Kubernetes. Just Git.

## How it works

```text
Scheduled run (cron / CI)
  → terraform plan on each stack
  → drift detected?
      NO  → silent, nothing happens
      YES → opens a PR/MR with plan diff + summary
```

## Install

**Homebrew (Mac/Linux):**

```bash
brew install MaripeddiSupraj/tap/terrawatch
```

**Binary download (Linux/Mac/Windows):**

Download the latest release from the [releases page](https://github.com/MaripeddiSupraj/terrawatch/releases) and place the binary on your `PATH`.

```bash
# Linux (amd64)
curl -sSL https://github.com/MaripeddiSupraj/terrawatch/releases/latest/download/terrawatch_linux_amd64.tar.gz | tar xz
sudo mv terrawatch /usr/local/bin/

# Mac (Apple Silicon)
curl -sSL https://github.com/MaripeddiSupraj/terrawatch/releases/latest/download/terrawatch_darwin_arm64.tar.gz | tar xz
sudo mv terrawatch /usr/local/bin/
```

**Go install:**

```bash
go install github.com/MaripeddiSupraj/terrawatch@latest
```

## Quick start

**1. Create a config file:**

For GitHub:

```yaml
# terrawatch.yaml
stacks:
  - name: production
    path: ./environments/prod
    vars_file: prod.tfvars     # optional
  - name: staging
    path: ./environments/staging

github:
  repo: your-org/your-infra-repo
  base_branch: main
  labels:
    - drift
    - infra

terraform:
  bin_path: terraform          # optional, defaults to terraform on PATH
```

For GitLab:

```yaml
# terrawatch.yaml
stacks:
  - name: production
    path: ./environments/prod
  - name: staging
    path: ./environments/staging

gitlab:
  repo: your-group/your-project
  url: https://gitlab.com      # optional — omit for gitlab.com
  base_branch: main
  labels:
    - drift

terraform:
  bin_path: terraform
```

**2. Run:**

```bash
# dry run — see what drifted, no PR/MR opened
GITHUB_TOKEN=xxx terrawatch detect --dry-run

# full run — opens a PR/MR per drifted stack
GITHUB_TOKEN=xxx terrawatch detect

# print version
terrawatch version
```

## CI integration

### GitHub Actions

Add the workflow to your infra repo. It runs daily at 06:00 UTC and can be triggered manually with an optional dry-run toggle.

```yaml
# .github/workflows/drift-detect.yml  (already included in this repo)
on:
  schedule:
    - cron: "0 6 * * *"
  workflow_dispatch:
    inputs:
      dry_run:
        type: boolean
```

Required secrets:

| Secret | Description |
| --- | --- |
| `TERRAWATCH_PAT` | PAT with `repo` scope — used to open PRs (recommended over GITHUB_TOKEN) |
| `AWS_ROLE_ARN` | IAM role to assume via OIDC (no stored keys) |
| `AWS_REGION` | AWS region (set as a Actions variable, not a secret) |

> **Why a PAT?** GitHub Actions' built-in `GITHUB_TOKEN` requires enabling "Allow GitHub Actions to create and approve pull requests" — a blanket repo setting. A dedicated PAT scoped to one token is safer for production.

### GitLab CI

`.gitlab-ci.yml` is included. Three behaviours:

| Trigger | Behaviour |
| --- | --- |
| Scheduled pipeline | Full detect — opens MR on drift |
| Manual (`web`) trigger | Full detect — opens MR on drift |
| Merge request pipeline | Dry run — prints drift, no MR opened |

Required CI/CD variables:

| Variable | Description |
| --- | --- |
| `GITLAB_TOKEN` | Personal access token with `api` scope |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | AWS credentials (or use OIDC) |

## PR/MR output

When drift is detected, terrawatch opens a PR (GitHub) or MR (GitLab) like this:

```text
[terrawatch] Drift detected in stack: production

Stack:      production
Path:       ./environments/prod
Detected:   Sun, 26 Apr 2026 06:00:00 UTC

Summary
| Add | Change | Destroy |
|  0  |   1    |    0    |

Plan
<details>
  # aws_instance.web will be updated in-place
  ~ instance_type = "t3.small" -> "t3.medium"
</details>
```

If an open drift PR/MR already exists for a stack, terrawatch skips creating a duplicate and returns the existing one.

## CLI output

```text
  terrawatch v0.1.0

  Scanning 2 stack(s)

  ✓  production          no drift
  ⚠  staging             drift detected  +1 ~0 -0

  ──────────────────────────────────────────────────
  2 scanned  ·  1 drifted  ·  1 clean

  Opening pull requests...

  ✓  staging             PR opened  →  https://github.com/.../pull/5
```

Colors are disabled automatically in CI and non-TTY environments.

## Configuration reference

```yaml
stacks:
  - name: string           # required — stack label
    path: string           # required — path to terraform root module
    vars_file: string      # optional — .tfvars file
    backend_config:        # optional — key/value backend overrides
      key: value

# Use either github OR gitlab — not both

github:
  token: string            # optional — defaults to GITHUB_TOKEN env var
  repo: owner/repo         # required
  base_branch: main        # optional — defaults to "main"
  labels: []               # optional — labels to add to the PR
  assignees: []            # optional — GitHub usernames to assign

gitlab:
  token: string            # optional — defaults to GITLAB_TOKEN env var
  repo: group/project      # required
  url: https://gitlab.com  # optional — for self-hosted GitLab
  base_branch: main        # optional — defaults to "main"
  labels: []               # optional — labels to add to the MR
  assignees: []            # optional — GitLab usernames to assign

terraform:
  bin_path: terraform      # optional — path to terraform binary
```

## Why terrawatch?

| | Atlantis | tf-controller | terrawatch |
| --- | --- | --- | --- |
| Requires server | Yes | Yes (K8s) | No |
| Stored credentials | Yes | Yes | No (OIDC) |
| Detects drift | No | No | Yes |
| Opens PR/MR automatically | Yes | No | Yes |
| GitHub + GitLab support | GitHub only | No | Yes |
| Open source | Yes | Yes | Yes |

## License

MIT
