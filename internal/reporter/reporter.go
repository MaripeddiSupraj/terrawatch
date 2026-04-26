package reporter

import (
	"context"

	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
)

// Reporter is implemented by GitHub and GitLab reporters.
type Reporter interface {
	CreateDriftPR(ctx context.Context, d detector.DriftResult) (*PRResult, error)
}
