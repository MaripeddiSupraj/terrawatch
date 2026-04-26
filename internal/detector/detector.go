package detector

import (
	"fmt"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

type DriftResult struct {
	Stack  config.Stack
	Plan       *terraform.PlanResult
	DetectedAt time.Time
}

type Detector struct {
	cfg         *config.Config
	plannerFunc func(ws config.Stack) terraform.Planner
}

func New(cfg *config.Config) *Detector {
	return &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			return terraform.New(cfg.Terraform.BinPath, ws.Path)
		},
	}
}

// Detect runs terraform plan across all stacks and returns those with drift.
func (d *Detector) Detect() ([]DriftResult, error) {
	var drifts []DriftResult

	for _, ws := range d.cfg.Stacks {
		result, err := d.checkStack(ws)
		if err != nil {
			return nil, fmt.Errorf("stack %q: %w", ws.Name, err)
		}
		if result != nil {
			drifts = append(drifts, *result)
		}
	}

	return drifts, nil
}

// DetectOne checks a single stack and returns nil if no drift.
func (d *Detector) DetectOne(ws config.Stack) (*DriftResult, error) {
	return d.checkStack(ws)
}

func (d *Detector) checkStack(ws config.Stack) (*DriftResult, error) {
	runner := d.plannerFunc(ws)

	if err := runner.Init(); err != nil {
		return nil, fmt.Errorf("init failed: %w", err)
	}

	plan, err := runner.Plan(ws.VarsFile)
	if err != nil {
		return nil, fmt.Errorf("plan failed: %w", err)
	}

	if !plan.HasChanges {
		return nil, nil
	}

	return &DriftResult{
		Stack:  ws,
		Plan:       plan,
		DetectedAt: time.Now().UTC(),
	}, nil
}
