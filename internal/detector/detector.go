package detector

import (
	"fmt"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/driftfilter"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

// Kind classifies what a detected change actually is.
type Kind string

const (
	// KindUnclassified means classification was not requested.
	KindUnclassified Kind = ""
	// KindInfraDrift means live infrastructure differs from state —
	// someone or something changed the cloud outside terraform.
	KindInfraDrift Kind = "infra_drift"
	// KindUnappliedChanges means the plan changes come from code that was
	// merged but never applied — the cloud still matches the state.
	KindUnappliedChanges Kind = "unapplied_changes"
)

type DriftResult struct {
	Stack         config.Stack
	Plan          *terraform.PlanResult
	DetectedAt    time.Time
	HiddenChanges int
	Kind          Kind
}

type Detector struct {
	cfg         *config.Config
	plannerFunc func(ws config.Stack) terraform.Planner
	// Classify runs an extra refresh-only plan per drifted stack to
	// distinguish real infra drift from unapplied code changes. It is
	// enabled by drift_mode: strict or the --classify flag.
	Classify bool
}

func New(cfg *config.Config) *Detector {
	timeout := cfg.TerraformTimeout(terraform.DefaultTimeout)
	return &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			return terraform.New(cfg.Terraform.BinPath, ws.Path).
				WithTimeout(timeout).
				WithBackendConfig(ws.BackendConfig)
		},
		Classify: cfg.DriftMode == config.DriftModeStrict,
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

// Outcome is the result of scanning one stack. Result is nil when the
// stack is clean.
type Outcome struct {
	Stack  config.Stack
	Result *DriftResult
	Err    error
}

// DetectAsync scans all configured stacks with up to workers concurrent
// planners. It returns one channel per stack, in stack order; each channel
// receives exactly one Outcome. Jobs are dispatched in stack order, so
// workers=1 behaves exactly like a sequential scan.
func (d *Detector) DetectAsync(workers int) []<-chan Outcome {
	stacks := d.cfg.Stacks
	if workers < 1 {
		workers = 1
	}
	if workers > len(stacks) {
		workers = len(stacks)
	}

	jobs := make(chan int)
	outs := make([]chan Outcome, len(stacks))
	ret := make([]<-chan Outcome, len(stacks))
	for i := range outs {
		outs[i] = make(chan Outcome, 1)
		ret[i] = outs[i]
	}

	for w := 0; w < workers; w++ {
		go func() {
			for i := range jobs {
				result, err := d.checkStack(stacks[i])
				outs[i] <- Outcome{Stack: stacks[i], Result: result, Err: err}
			}
		}()
	}
	go func() {
		for i := range stacks {
			jobs <- i
		}
		close(jobs)
	}()

	return ret
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

	// Apply ignore rules to reduce noise.
	hidden := 0
	if len(d.cfg.Ignore) > 0 || len(ws.Ignore) > 0 {
		filtered := driftfilter.Apply(plan.ResourceChanges, d.cfg.Ignore, ws.Ignore)
		hidden = filtered.HiddenChanges

		if len(filtered.Changes) < len(plan.ResourceChanges) {
			newSummary := driftfilter.ComputeSummary(filtered.Changes)
			plan.Summary = newSummary
			plan.ResourceChanges = filtered.Changes
			plan.HasChanges = len(filtered.Changes) > 0
		}
	}

	if !plan.HasChanges {
		return nil, nil
	}

	kind := KindUnclassified
	if d.Classify {
		refresh, err := runner.PlanRefreshOnly(ws.VarsFile)
		if err != nil {
			return nil, fmt.Errorf("refresh-only plan failed: %w", err)
		}
		if refresh.HasChanges {
			kind = KindInfraDrift
		} else {
			kind = KindUnappliedChanges
		}
	}

	return &DriftResult{
		Stack:         ws,
		Plan:          plan,
		DetectedAt:    time.Now().UTC(),
		HiddenChanges: hidden,
		Kind:          kind,
	}, nil
}
