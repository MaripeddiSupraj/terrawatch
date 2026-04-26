package detector

import (
	"errors"
	"testing"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

type mockPlanner struct {
	initErr error
	result  *terraform.PlanResult
	planErr error
}

func (m *mockPlanner) Init() error                              { return m.initErr }
func (m *mockPlanner) Plan(_ string) (*terraform.PlanResult, error) { return m.result, m.planErr }

func testConfig(stacks ...config.Stack) *config.Config {
	return &config.Config{
		Stacks: stacks,
		GitHub:     config.GitHub{Token: "tok", Repo: "org/repo", BaseBranch: "main"},
	}
}

func newDetectorWithMock(cfg *config.Config, planner terraform.Planner) *Detector {
	return &Detector{
		cfg: cfg,
		plannerFunc: func(_ config.Stack) terraform.Planner {
			return planner
		},
	}
}

func TestDetect_no_drift(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	d := newDetectorWithMock(cfg, &mockPlanner{
		result: &terraform.PlanResult{HasChanges: false},
	})

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(drifts) != 0 {
		t.Errorf("expected no drifts, got %d", len(drifts))
	}
}

func TestDetect_with_drift(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	d := newDetectorWithMock(cfg, &mockPlanner{
		result: &terraform.PlanResult{
			HasChanges: true,
			Output:     "~ aws_instance.web",
			Summary:    terraform.Summary{Change: 1},
		},
	})

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift, got %d", len(drifts))
	}
	if drifts[0].Stack.Name != "prod" {
		t.Errorf("expected stack 'prod', got %q", drifts[0].Stack.Name)
	}
	if drifts[0].Plan.Summary.Change != 1 {
		t.Errorf("expected Change=1, got %d", drifts[0].Plan.Summary.Change)
	}
	if drifts[0].DetectedAt.IsZero() {
		t.Error("expected DetectedAt to be set")
	}
}

func TestDetect_multiple_stacks(t *testing.T) {
	cfg := testConfig(
		config.Stack{Name: "dev", Path: "./dev"},
		config.Stack{Name: "prod", Path: "./prod"},
	)
	calls := 0
	d := &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			calls++
			hasDrift := ws.Name == "prod"
			return &mockPlanner{
				result: &terraform.PlanResult{HasChanges: hasDrift},
			}
		},
	}

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 planner calls, got %d", calls)
	}
	if len(drifts) != 1 {
		t.Fatalf("expected 1 drift (prod only), got %d", len(drifts))
	}
	if drifts[0].Stack.Name != "prod" {
		t.Errorf("expected drift in 'prod', got %q", drifts[0].Stack.Name)
	}
}

func TestDetect_init_error(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	d := newDetectorWithMock(cfg, &mockPlanner{
		initErr: errors.New("backend unreachable"),
	})

	_, err := d.Detect()
	if err == nil {
		t.Fatal("expected error from init failure")
	}
}

func TestDetect_plan_error(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	d := newDetectorWithMock(cfg, &mockPlanner{
		planErr: errors.New("provider error"),
	})

	_, err := d.Detect()
	if err == nil {
		t.Fatal("expected error from plan failure")
	}
}

func TestDetect_detected_at_is_utc(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	d := newDetectorWithMock(cfg, &mockPlanner{
		result: &terraform.PlanResult{HasChanges: true},
	})

	drifts, _ := d.Detect()
	if drifts[0].DetectedAt.Location() != time.UTC {
		t.Error("expected DetectedAt in UTC")
	}
}
