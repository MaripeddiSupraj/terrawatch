package reporter

import (
	"context"

	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

// Reporter is implemented by GitHub and GitLab reporters.
type Reporter interface {
	CreateDriftPR(ctx context.Context, d detector.DriftResult) (*PRResult, error)
	// CloseResolvedDriftPR closes the open drift PR/MR for a stack that is
	// clean again, commenting first and deleting the branch. It returns
	// (nil, nil) when no open drift PR exists for the stack.
	CloseResolvedDriftPR(ctx context.Context, stackName string) (*PRResult, error)
	// ValidateAuth performs one cheap authenticated API call so preflight
	// checks can verify the configured token actually works.
	ValidateAuth(ctx context.Context) error
}
