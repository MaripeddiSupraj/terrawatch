package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "terrawatch-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}

const validYAML = `
stacks:
  - name: production
    path: ./environments/prod
    vars_file: prod.tfvars
github:
  token: ghp_test123
  repo: org/repo
  base_branch: main
  labels:
    - drift
terraform:
  bin_path: terraform
`

func TestLoad_valid(t *testing.T) {
	cfg, err := Load(writeTemp(t, validYAML))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Stacks) != 1 {
		t.Fatalf("expected 1 stack, got %d", len(cfg.Stacks))
	}
	if cfg.Stacks[0].Name != "production" {
		t.Errorf("expected stack name 'production', got %q", cfg.Stacks[0].Name)
	}
	if cfg.GitHub.Repo != "org/repo" {
		t.Errorf("expected repo 'org/repo', got %q", cfg.GitHub.Repo)
	}
	if len(cfg.GitHub.Labels) != 1 || cfg.GitHub.Labels[0] != "drift" {
		t.Errorf("expected labels [drift], got %v", cfg.GitHub.Labels)
	}
}

func TestLoad_default_base_branch(t *testing.T) {
	yaml := `
stacks:
  - name: dev
    path: ./dev
github:
  token: tok
  repo: org/repo
`
	cfg, err := Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHub.BaseBranch != "main" {
		t.Errorf("expected default base_branch 'main', got %q", cfg.GitHub.BaseBranch)
	}
}

func TestLoad_github_token_from_env(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token-xyz")
	yaml := `
stacks:
  - name: dev
    path: ./dev
github:
  repo: org/repo
`
	cfg, err := Load(writeTemp(t, yaml))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitHub.Token != "env-token-xyz" {
		t.Errorf("expected token from env, got %q", cfg.GitHub.Token)
	}
}

func TestLoad_missing_stacks(t *testing.T) {
	yaml := `
github:
  token: tok
  repo: org/repo
`
	_, err := Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing stacks")
	}
}

func TestLoad_missing_repo(t *testing.T) {
	yaml := `
stacks:
  - name: dev
    path: ./dev
github:
  token: tok
`
	_, err := Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
}

func TestLoad_missing_token(t *testing.T) {
	os.Unsetenv("GITHUB_TOKEN")
	yaml := `
stacks:
  - name: dev
    path: ./dev
github:
  repo: org/repo
`
	_, err := Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestLoad_stack_missing_path(t *testing.T) {
	yaml := `
stacks:
  - name: dev
github:
  token: tok
  repo: org/repo
`
	_, err := Load(writeTemp(t, yaml))
	if err == nil {
		t.Fatal("expected error for stack missing path")
	}
}

func TestLoad_file_not_found(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
