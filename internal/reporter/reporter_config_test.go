package reporter

import (
	"testing"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
)

func TestNewGitHub_requires_token(t *testing.T) {
	_, err := NewGitHub(config.GitHub{Repo: "org/repo", BaseBranch: "main"})
	if err == nil {
		t.Fatal("expected missing GitHub token to fail")
	}
}

func TestNewGitHub_rejects_invalid_repo(t *testing.T) {
	cases := []string{"", "owner", "/repo", "owner/", "owner/repo/extra"}
	for _, repo := range cases {
		t.Run(repo, func(t *testing.T) {
			_, err := NewGitHub(config.GitHub{Token: "token", Repo: repo, BaseBranch: "main"})
			if err == nil {
				t.Fatalf("expected repo %q to be rejected", repo)
			}
		})
	}
}

func TestNewGitLab_requires_token(t *testing.T) {
	_, err := NewGitLab(config.GitLab{Repo: "group/project", BaseURL: "https://gitlab.com", BaseBranch: "main"})
	if err == nil {
		t.Fatal("expected missing GitLab token to fail")
	}
}
