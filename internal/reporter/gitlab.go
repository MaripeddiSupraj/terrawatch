package reporter

import (
	"context"
	"fmt"
	"strings"

	gl "gitlab.com/gitlab-org/api/client-go"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

type GitLab struct {
	client  *gl.Client
	project string
	cfg     config.GitLab
}

func NewGitLab(cfg config.GitLab) (*GitLab, error) {
	if cfg.Repo == "" {
		return nil, fmt.Errorf("gitlab.repo is required")
	}
	client, err := gl.NewClient(cfg.Token, gl.WithBaseURL(cfg.BaseURL))
	if err != nil {
		return nil, fmt.Errorf("gitlab client: %w", err)
	}
	return &GitLab{client: client, project: cfg.Repo, cfg: cfg}, nil
}

// CreateDriftPR creates a GitLab MR for drift, or comments on the existing open one.
func (g *GitLab) CreateDriftPR(ctx context.Context, d detector.DriftResult) (*PRResult, error) {
	if existing, err := g.findExistingMR(d.Stack.Name); err != nil {
		return nil, fmt.Errorf("check existing MRs: %w", err)
	} else if existing != nil {
		_ = g.addMRComment(existing.Number, commentBody(d))
		return existing, nil
	}

	branch := branchName(d.Stack.Name, d.DetectedAt)
	content := reportFileContent(d)
	filename := reportFilename(d.Stack.Name, d.DetectedAt)

	if err := g.createBranch(branch); err != nil {
		return nil, fmt.Errorf("create branch: %w", err)
	}
	if err := g.createFile(branch, filename, content, d); err != nil {
		return nil, fmt.Errorf("create file: %w", err)
	}
	return g.openMR(branch, d)
}

func (g *GitLab) findExistingMR(stackName string) (*PRResult, error) {
	state := "opened"
	opts := &gl.ListProjectMergeRequestsOptions{
		State:        &state,
		TargetBranch: &g.cfg.BaseBranch,
		ListOptions:  gl.ListOptions{PerPage: 100},
	}
	title := prTitle(stackName)
	for {
		mrs, resp, err := g.client.MergeRequests.ListProjectMergeRequests(g.project, opts)
		if err != nil {
			return nil, err
		}
		for _, mr := range mrs {
			if mr.Title == title {
				return &PRResult{URL: mr.WebURL, Number: int(mr.IID), Existing: true}, nil
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return nil, nil
}

func (g *GitLab) createBranch(branch string) error {
	_, _, err := g.client.Branches.CreateBranch(g.project, &gl.CreateBranchOptions{
		Branch: &branch,
		Ref:    &g.cfg.BaseBranch,
	})
	return err
}

func (g *GitLab) createFile(branch, filename, content string, d detector.DriftResult) error {
	msg := fmt.Sprintf("chore: drift report for %s at %s", d.Stack.Name, d.DetectedAt.Format("2006-01-02 15:04:05 UTC"))
	_, _, err := g.client.RepositoryFiles.CreateFile(g.project, filename, &gl.CreateFileOptions{
		Branch:        &branch,
		CommitMessage: &msg,
		Content:       &content,
	})
	return err
}

func (g *GitLab) openMR(branch string, d detector.DriftResult) (*PRResult, error) {
	title := prTitle(d.Stack.Name)
	body := prBody(d)

	opts := &gl.CreateMergeRequestOptions{
		Title:        &title,
		Description:  &body,
		SourceBranch: &branch,
		TargetBranch: &g.cfg.BaseBranch,
	}

	if len(g.cfg.Labels) > 0 {
		labels := gl.LabelOptions(g.cfg.Labels)
		opts.Labels = &labels
	}

	if len(g.cfg.Assignees) > 0 {
		ids, err := g.resolveUserIDs(g.cfg.Assignees)
		if err == nil {
			opts.AssigneeIDs = &ids
		}
	}

	mr, _, err := g.client.MergeRequests.CreateMergeRequest(g.project, opts)
	if err != nil {
		return nil, err
	}
	return &PRResult{URL: mr.WebURL, Number: int(mr.IID)}, nil
}

func (g *GitLab) addMRComment(mrIID int, body string) error {
	_, _, err := g.client.Notes.CreateMergeRequestNote(g.project, int64(mrIID), &gl.CreateMergeRequestNoteOptions{
		Body: &body,
	})
	return err
}

func (g *GitLab) resolveUserIDs(usernames []string) ([]int64, error) {
	var ids []int64
	for _, username := range usernames {
		users, _, err := g.client.Users.ListUsers(&gl.ListUsersOptions{
			Username: &username,
		})
		if err != nil || len(users) == 0 {
			continue
		}
		ids = append(ids, int64(users[0].ID))
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no users resolved from %s", strings.Join(usernames, ", "))
	}
	return ids, nil
}
