package reporter

import (
	"context"
	"fmt"
	"strings"

	gogithub "github.com/google/go-github/v62/github"
	"golang.org/x/oauth2"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

type GitHub struct {
	client *gogithub.Client
	owner  string
	repo   string
	cfg    config.GitHub
}

type PRResult struct {
	URL      string
	Number   int
	Existing bool // true if PR already existed
}

func ptr[T any](v T) *T { return &v }

func NewGitHub(cfg config.GitHub) (*GitHub, error) {
	parts := strings.SplitN(cfg.Repo, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format %q, expected owner/repo", cfg.Repo)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: cfg.Token})
	tc := oauth2.NewClient(context.Background(), ts)
	client := gogithub.NewClient(tc)

	return &GitHub{
		client: client,
		owner:  parts[0],
		repo:   parts[1],
		cfg:    cfg,
	}, nil
}

// CreateDriftPR creates a PR for drift, or returns the existing open one if found.
func (g *GitHub) CreateDriftPR(ctx context.Context, d detector.DriftResult) (*PRResult, error) {
	// check for an already-open drift PR for this stack
	if existing, err := g.findExistingDriftPR(ctx, d.Stack.Name); err != nil {
		return nil, fmt.Errorf("check existing PRs: %w", err)
	} else if existing != nil {
		return existing, nil
	}

	branch := branchName(d.Stack.Name, d.DetectedAt)
	filename := reportFilename(d.Stack.Name, d.DetectedAt)
	content := reportFileContent(d)

	baseSHA, err := g.getBaseSHA(ctx)
	if err != nil {
		return nil, fmt.Errorf("get base SHA: %w", err)
	}
	if err := g.createBranch(ctx, branch, baseSHA); err != nil {
		return nil, fmt.Errorf("create branch: %w", err)
	}
	if err := g.createFile(ctx, branch, filename, content, d); err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	pr, err := g.openPR(ctx, branch, d)
	if err != nil {
		return nil, fmt.Errorf("open PR: %w", err)
	}
	return pr, nil
}

// findExistingDriftPR searches open PRs for one already tracking this stack.
func (g *GitHub) findExistingDriftPR(ctx context.Context, stackName string) (*PRResult, error) {
	opts := &gogithub.PullRequestListOptions{
		State:       "open",
		Base:        g.cfg.BaseBranch,
		ListOptions: gogithub.ListOptions{PerPage: 100},
	}
	expectedTitle := prTitle(stackName)
	for {
		prs, resp, err := g.client.PullRequests.List(ctx, g.owner, g.repo, opts)
		if err != nil {
			return nil, err
		}
		for _, pr := range prs {
			if pr.GetTitle() == expectedTitle {
				return &PRResult{
					URL:      pr.GetHTMLURL(),
					Number:   pr.GetNumber(),
					Existing: true,
				}, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil, nil
}

func (g *GitHub) getBaseSHA(ctx context.Context) (string, error) {
	ref, _, err := g.client.Git.GetRef(ctx, g.owner, g.repo, "refs/heads/"+g.cfg.BaseBranch)
	if err != nil {
		return "", err
	}
	return ref.GetObject().GetSHA(), nil
}

func (g *GitHub) createBranch(ctx context.Context, branch, sha string) error {
	ref := &gogithub.Reference{
		Ref:    ptr("refs/heads/" + branch),
		Object: &gogithub.GitObject{SHA: ptr(sha)},
	}
	_, _, err := g.client.Git.CreateRef(ctx, g.owner, g.repo, ref)
	return err
}

func (g *GitHub) createFile(ctx context.Context, branch, filename, content string, d detector.DriftResult) error {
	msg := fmt.Sprintf("chore: drift report for %s at %s", d.Stack.Name, d.DetectedAt.Format("2006-01-02 15:04:05 UTC"))
	opts := &gogithub.RepositoryContentFileOptions{
		Message: ptr(msg),
		Content: []byte(content),
		Branch:  ptr(branch),
	}
	_, _, err := g.client.Repositories.CreateFile(ctx, g.owner, g.repo, filename, opts)
	return err
}

func (g *GitHub) openPR(ctx context.Context, branch string, d detector.DriftResult) (*PRResult, error) {
	newPR := &gogithub.NewPullRequest{
		Title: ptr(prTitle(d.Stack.Name)),
		Head:  ptr(branch),
		Base:  ptr(g.cfg.BaseBranch),
		Body:  ptr(prBody(d)),
	}
	pr, _, err := g.client.PullRequests.Create(ctx, g.owner, g.repo, newPR)
	if err != nil {
		return nil, err
	}
	if len(g.cfg.Labels) > 0 {
		_, _, _ = g.client.Issues.AddLabelsToIssue(ctx, g.owner, g.repo, pr.GetNumber(), g.cfg.Labels)
	}
	if len(g.cfg.Assignees) > 0 {
		_, _, _ = g.client.Issues.AddAssignees(ctx, g.owner, g.repo, pr.GetNumber(), g.cfg.Assignees)
	}
	return &PRResult{URL: pr.GetHTMLURL(), Number: pr.GetNumber()}, nil
}
