package reporter

import (
	"strings"
	"testing"
	"time"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

var fixedTime = time.Date(2026, 4, 23, 6, 0, 0, 0, time.UTC)

func TestBranchName(t *testing.T) {
	got := branchName("production", fixedTime)
	want := "drift/production-20260423-060000"
	if got != want {
		t.Errorf("branchName = %q, want %q", got, want)
	}
}

func TestBranchName_special_chars(t *testing.T) {
	got := branchName("my-stack", fixedTime)
	if !strings.HasPrefix(got, "drift/my-stack-") {
		t.Errorf("branchName %q missing expected prefix", got)
	}
}

func TestReportFilename(t *testing.T) {
	got := reportFilename("staging", fixedTime)
	want := "drift-reports/staging-20260423-060000.md"
	if got != want {
		t.Errorf("reportFilename = %q, want %q", got, want)
	}
}

func TestPrTitle(t *testing.T) {
	got := prTitle("production")
	want := "[terrawatch] Drift detected in stack: production"
	if got != want {
		t.Errorf("prTitle = %q, want %q", got, want)
	}
}

func makeDriftResult() detector.DriftResult {
	return detector.DriftResult{
		Stack: config.Stack{Name: "production", Path: "./environments/prod"},
		Plan: &terraform.PlanResult{
			HasChanges: true,
			Output:     "~ aws_instance.web",
			Summary:    terraform.Summary{Add: 0, Change: 1, Destroy: 0},
		},
		DetectedAt: fixedTime,
	}
}

func TestPrBody_contains_stack(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "production") {
		t.Error("PR body missing stack name")
	}
}

func TestPrBody_contains_path(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "./environments/prod") {
		t.Error("PR body missing stack path")
	}
}

func TestPrBody_contains_summary_table(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "| Add | Change | Destroy |") {
		t.Error("PR body missing summary table header")
	}
	if !strings.Contains(body, "| 0 | 1 | 0 |") {
		t.Error("PR body missing summary values")
	}
}

func TestPrBody_contains_plan_output(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "aws_instance.web") {
		t.Error("PR body missing plan output")
	}
}

func TestPrBody_contains_terrawatch_link(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "terrawatch") {
		t.Error("PR body missing terrawatch attribution")
	}
}

func TestPrBody_plan_in_details_block(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "<details>") {
		t.Error("PR body plan output should be in a details block")
	}
}

func TestDriftBranchPrefix_matches_branchName(t *testing.T) {
	// the auto-close safety gate keys off this prefix; it must match
	// what branchName actually produces
	if !strings.HasPrefix(branchName("any", fixedTime), driftBranchPrefix) {
		t.Errorf("branchName output must start with driftBranchPrefix %q", driftBranchPrefix)
	}
}

func TestPRBody_infra_drift_kind(t *testing.T) {
	d := detector.DriftResult{
		Stack:      config.Stack{Name: "eks", Path: "./eks"},
		Plan:       &terraform.PlanResult{Summary: terraform.Summary{Change: 1}, Output: "~ x"},
		DetectedAt: fixedTime,
		Kind:       detector.KindInfraDrift,
	}
	body := prBody(d)
	if !strings.Contains(body, "Real infrastructure drift") {
		t.Errorf("expected infra-drift callout in PR body, got:\n%s", body)
	}
}

func TestPRBody_unapplied_kind(t *testing.T) {
	d := detector.DriftResult{
		Stack:      config.Stack{Name: "eks", Path: "./eks"},
		Plan:       &terraform.PlanResult{Summary: terraform.Summary{Add: 1}, Output: "+ x"},
		DetectedAt: fixedTime,
		Kind:       detector.KindUnappliedChanges,
	}
	body := prBody(d)
	if !strings.Contains(body, "Unapplied code changes") {
		t.Errorf("expected unapplied-changes callout in PR body, got:\n%s", body)
	}
}
