# terrawatch

Detect Terraform infrastructure drift and automatically open a pull request to fix it.

## How it works

```
Scheduled run (cron / CI)
  → terraform plan
  → drift detected?
      NO  → silent, nothing happens
      YES → opens a PR with the plan diff
```

## Quick start

```bash
terrawatch detect --config terrawatch.yaml
```

## Config

```yaml
# terrawatch.yaml
workspaces:
  - name: production
    path: ./environments/prod
    backend: s3

github:
  repo: org/infra-repo
  base_branch: main

notify:
  labels: ["drift", "infra"]
```

## Status

Work in progress.
