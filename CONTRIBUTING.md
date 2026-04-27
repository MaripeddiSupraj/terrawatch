# Contributing to terrawatch

Thanks for taking the time to contribute. This document covers everything you need to get started.

## Table of contents

- [Getting started](#getting-started)
- [Project structure](#project-structure)
- [Making changes](#making-changes)
- [Tests](#tests)
- [Submitting a pull request](#submitting-a-pull-request)
- [Reporting bugs](#reporting-bugs)

---

## Getting started

**Prerequisites:**

- Go 1.22+
- Terraform 1.5+ on your `PATH`
- A GitHub or GitLab account for testing PR/MR creation

**Clone and build:**

```bash
git clone https://github.com/MaripeddiSupraj/terrawatch.git
cd terrawatch
go mod tidy
go build -o terrawatch .
./terrawatch --help
```

**Run tests:**

```bash
go test ./...
```

---

## Project structure

```
terrawatch/
├── cmd/                    # CLI commands (cobra)
│   ├── root.go             # root command, version flag
│   └── detect.go           # detect subcommand
├── internal/
│   ├── config/             # config loading and validation
│   ├── detector/           # runs terraform plan, collects drift
│   ├── reporter/           # GitHub PR and GitLab MR creation
│   └── ui/                 # colored terminal output
├── pkg/
│   └── terraform/          # terraform binary wrapper
├── testdata/
│   └── workspace/          # null provider workspace for integration tests
├── .github/
│   └── workflows/          # CI, release, drift-detect, integration-test
├── .goreleaser.yaml        # release configuration
└── terrawatch.example.yaml # example config
```

---

## Making changes

**Branching:**

```bash
git checkout -b feat/your-feature   # new feature
git checkout -b fix/your-bug        # bug fix
```

**Key conventions:**

- New reporter platforms (e.g. Bitbucket) implement the `reporter.Reporter` interface in `internal/reporter/`
- Config changes go in `internal/config/config.go` — add validation in `validate()`
- CLI output always goes through `internal/ui` — never `fmt.Print` directly in commands
- `pkg/terraform` must stay free of business logic — it only wraps the terraform binary

---

## Tests

Every change should include tests. The project has two levels:

**Unit tests** — fast, no external dependencies:

```bash
go test ./...
```

**Integration test** — requires terraform on PATH, tests real drift detection:

```bash
cd testdata/workspace
terraform init
terraform apply -auto-approve -var="instance_count=1"
echo 'instance_count = 2' > terraform.tfvars
cd ../..
GITHUB_TOKEN=xxx go run . detect --config terrawatch-test.yaml --dry-run
```

When adding a new feature, please cover:
- Happy path
- Error cases (init failure, plan failure, API errors)
- Edge cases specific to your change

---

## Submitting a pull request

1. Fork the repo and create your branch from `main`
2. Make your changes and add tests
3. Run `go test ./...` and ensure all tests pass
4. Run `go build ./...` to confirm it compiles
5. Open a PR against `main` — fill in the PR template

**What makes a good PR:**
- One focused change per PR
- Tests included
- No unrelated refactoring mixed in

---

## Reporting bugs

Use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md) — it asks for the right information upfront so we can reproduce and fix faster.

---

## Questions?

Open a [discussion](https://github.com/MaripeddiSupraj/terrawatch/discussions) or a plain issue if something is unclear.
