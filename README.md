# terrawatch

Detect Terraform infrastructure drift and automatically open a pull request to fix it.

No servers. No Kubernetes. Just Git.

## How it works

```text
Scheduled run (cron / CI)
  → terraform plan on each workspace
  → drift detected?
      NO  → silent, nothing happens
      YES → opens a PR with plan diff + summary
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

**2. Run:**

```bash
# dry run — see what drifted, no PR opened
GITHUB_TOKEN=xxx terrawatch detect --dry-run

# full run — opens a PR per drifted workspace
GITHUB_TOKEN=xxx terrawatch detect
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
| `AWS_ROLE_ARN` | IAM role to assume via OIDC (no stored keys) |
| `AWS_REGION` | AWS region (set as a variable, not a secret) |
| `GITHUB_TOKEN` | Auto-provided by GitHub Actions |

### GitLab CI

`.gitlab-ci.yml` is included. Three behaviours:

| Trigger | Behaviour |
| --- | --- |
| Scheduled pipeline | Full detect — opens MR on drift |
| Manual (`web`) trigger | Full detect — opens MR on drift |
| Merge request pipeline | Dry run — prints drift, no MR opened |

## PR output

When drift is detected, terrawatch opens a PR like this:

```text
[terrawatch] Drift detected in workspace: production

Workspace: production
Path:      ./environments/prod
Detected:  Wed, 23 Apr 2026 06:00:00 UTC

Summary
| Add | Change | Destroy |
|  0  |   1    |    0    |

Plan
<details>
  # aws_instance.web will be updated in-place
  ~ instance_type = "t3.small" -> "t3.medium"
</details>
```

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
| Opens PR automatically | Yes | No | Yes |
| Open source | Yes | Yes | Yes |

## License

MIT
