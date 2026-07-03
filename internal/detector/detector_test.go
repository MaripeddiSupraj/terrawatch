package detector

import (
	"errors"
	"sync/atomic"
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

// slowPlanner blocks in Plan long enough for concurrency to be observable
// and tracks the peak number of in-flight plans.
type slowPlanner struct {
	inFlight *atomic.Int32
	peak     *atomic.Int32
	hasDrift bool
}

func (p *slowPlanner) Init() error { return nil }
func (p *slowPlanner) Plan(_ string) (*terraform.PlanResult, error) {
	n := p.inFlight.Add(1)
	for {
		old := p.peak.Load()
		if n <= old || p.peak.CompareAndSwap(old, n) {
			break
		}
	}
	time.Sleep(30 * time.Millisecond)
	p.inFlight.Add(-1)
	return &terraform.PlanResult{HasChanges: p.hasDrift}, nil
}
func (p *slowPlanner) PlanRefreshOnly(_ string) (*terraform.PlanResult, error) {
	return &terraform.PlanResult{}, nil
}

func TestDetectAsync_results_in_stack_order(t *testing.T) {
	cfg := testConfig(
		config.Stack{Name: "a", Path: "./a"},
		config.Stack{Name: "b", Path: "./b"},
		config.Stack{Name: "c", Path: "./c"},
	)
	var inFlight, peak atomic.Int32
	d := &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			return &slowPlanner{inFlight: &inFlight, peak: &peak, hasDrift: ws.Name == "b"}
		},
	}

	outcomes := d.DetectAsync(3)
	if len(outcomes) != 3 {
		t.Fatalf("expected 3 outcome channels, got %d", len(outcomes))
	}
	for i, name := range []string{"a", "b", "c"} {
		oc := <-outcomes[i]
		if oc.Stack.Name != name {
			t.Errorf("outcome %d: expected stack %q, got %q", i, name, oc.Stack.Name)
		}
		if oc.Err != nil {
			t.Errorf("outcome %d: unexpected error: %v", i, oc.Err)
		}
		wantDrift := name == "b"
		if (oc.Result != nil) != wantDrift {
			t.Errorf("outcome %d (%s): drift=%v, want %v", i, name, oc.Result != nil, wantDrift)
		}
	}
	if peak.Load() < 2 {
		t.Errorf("expected at least 2 concurrent plans with 3 workers, peak was %d", peak.Load())
	}
}

func TestDetectAsync_one_worker_is_sequential(t *testing.T) {
	cfg := testConfig(
		config.Stack{Name: "a", Path: "./a"},
		config.Stack{Name: "b", Path: "./b"},
		config.Stack{Name: "c", Path: "./c"},
	)
	var inFlight, peak atomic.Int32
	d := &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			return &slowPlanner{inFlight: &inFlight, peak: &peak}
		},
	}

	// workers=0 must clamp to 1, so both runs stay strictly sequential
	for _, workers := range []int{1, 0} {
		outcomes := d.DetectAsync(workers)
		for i := range outcomes {
			<-outcomes[i]
		}
		if peak.Load() != 1 {
			t.Errorf("workers=%d: expected exactly 1 in-flight plan, peak was %d", workers, peak.Load())
		}
	}
}

func TestDetectAsync_error_propagates_per_stack(t *testing.T) {
	cfg := testConfig(
		config.Stack{Name: "ok", Path: "./ok"},
		config.Stack{Name: "bad", Path: "./bad"},
	)
	d := &Detector{
		cfg: cfg,
		plannerFunc: func(ws config.Stack) terraform.Planner {
			if ws.Name == "bad" {
				return &mockPlanner{planErr: errors.New("provider exploded")}
			}
			return &mockPlanner{result: &terraform.PlanResult{HasChanges: false}}
		},
	}

	outcomes := d.DetectAsync(2)
	if oc := <-outcomes[0]; oc.Err != nil {
		t.Errorf("stack ok: unexpected error: %v", oc.Err)
	}
	if oc := <-outcomes[1]; oc.Err == nil {
		t.Error("stack bad: expected plan error to propagate")
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
