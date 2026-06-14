package detector

import (
	"errors"
	"testing"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

type mockPlanner struct {
	initErr       error
	result        *terraform.PlanResult
	planErr       error
	refreshResult *terraform.PlanResult
	refreshErr    error
	refreshCalls  int
}

func (m *mockPlanner) Init() error                                  { return m.initErr }
func (m *mockPlanner) Plan(_ string) (*terraform.PlanResult, error) { return m.result, m.planErr }
func (m *mockPlanner) PlanRefreshOnly(_ string) (*terraform.PlanResult, error) {
	m.refreshCalls++
	if m.refreshResult == nil {
		return &terraform.PlanResult{}, m.refreshErr
	}
	return m.refreshResult, m.refreshErr
}

func testConfig(stacks ...config.Stack) *config.Config {
	return &config.Config{
		Stacks: stacks,
		GitHub: config.GitHub{Token: "tok", Repo: "org/repo", BaseBranch: "main"},
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

func TestClassify_off_by_default(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	m := &mockPlanner{result: &terraform.PlanResult{HasChanges: true}}
	d := newDetectorWithMock(cfg, m)

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.refreshCalls != 0 {
		t.Errorf("expected no refresh-only plan without classify, got %d calls", m.refreshCalls)
	}
	if drifts[0].Kind != KindUnclassified {
		t.Errorf("expected unclassified kind, got %q", drifts[0].Kind)
	}
}

func TestClassify_infra_drift(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	m := &mockPlanner{
		result:        &terraform.PlanResult{HasChanges: true},
		refreshResult: &terraform.PlanResult{HasChanges: true},
	}
	d := newDetectorWithMock(cfg, m)
	d.Classify = true

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.refreshCalls != 1 {
		t.Errorf("expected 1 refresh-only call, got %d", m.refreshCalls)
	}
	if drifts[0].Kind != KindInfraDrift {
		t.Errorf("expected infra_drift, got %q", drifts[0].Kind)
	}
}

func TestClassify_unapplied_changes(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	m := &mockPlanner{
		result:        &terraform.PlanResult{HasChanges: true},
		refreshResult: &terraform.PlanResult{HasChanges: false},
	}
	d := newDetectorWithMock(cfg, m)
	d.Classify = true

	drifts, err := d.Detect()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if drifts[0].Kind != KindUnappliedChanges {
		t.Errorf("expected unapplied_changes, got %q", drifts[0].Kind)
	}
}

func TestClassify_skips_refresh_when_plan_clean(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	m := &mockPlanner{result: &terraform.PlanResult{HasChanges: false}}
	d := newDetectorWithMock(cfg, m)
	d.Classify = true

	if _, err := d.Detect(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.refreshCalls != 0 {
		t.Errorf("clean stack must not trigger a refresh-only plan, got %d calls", m.refreshCalls)
	}
}

func TestClassify_refresh_error_propagates(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	m := &mockPlanner{
		result:     &terraform.PlanResult{HasChanges: true},
		refreshErr: errors.New("provider crashed"),
	}
	d := newDetectorWithMock(cfg, m)
	d.Classify = true

	if _, err := d.Detect(); err == nil {
		t.Fatal("expected refresh-only failure to propagate")
	}
}

func TestNew_strict_mode_enables_classify(t *testing.T) {
	cfg := testConfig(config.Stack{Name: "prod", Path: "./prod"})
	cfg.DriftMode = config.DriftModeStrict
	if !New(cfg).Classify {
		t.Error("drift_mode strict must enable classification")
	}
	cfg.DriftMode = config.DriftModeAll
	if New(cfg).Classify {
		t.Error("drift_mode all must not enable classification")
	}
}
