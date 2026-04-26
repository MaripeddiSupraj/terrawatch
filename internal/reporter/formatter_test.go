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
	got := branchName("my-workspace", fixedTime)
	if !strings.HasPrefix(got, "drift/my-workspace-") {
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
	want := "[terrawatch] Drift detected in workspace: production"
	if got != want {
		t.Errorf("prTitle = %q, want %q", got, want)
	}
}

func makeDriftResult() detector.DriftResult {
	return detector.DriftResult{
		Workspace: config.Workspace{Name: "production", Path: "./environments/prod"},
		Plan: &terraform.PlanResult{
			HasChanges: true,
			Output:     "~ aws_instance.web",
			Summary:    terraform.Summary{Add: 0, Change: 1, Destroy: 0},
		},
		DetectedAt: fixedTime,
	}
}

func TestPrBody_contains_workspace(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "production") {
		t.Error("PR body missing workspace name")
	}
}

func TestPrBody_contains_path(t *testing.T) {
	body := prBody(makeDriftResult())
	if !strings.Contains(body, "./environments/prod") {
		t.Error("PR body missing workspace path")
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
