package terraform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// fakeBin creates an executable file named name in dir.
func fakeBin(t *testing.T, dir, name string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestResolveBinPath_configured(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binaries")
	}
	dir := t.TempDir()
	fakeBin(t, dir, "my-terraform")
	got, err := ResolveBinPath(filepath.Join(dir, "my-terraform"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != filepath.Join(dir, "my-terraform") {
		t.Errorf("expected configured path back, got %q", got)
	}
}

func TestResolveBinPath_configured_missing(t *testing.T) {
	_, err := ResolveBinPath("/nonexistent/terraform-xyz")
	if err == nil {
		t.Fatal("expected error for missing configured binary")
	}
}

func TestResolveBinPath_prefers_terraform(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binaries")
	}
	dir := t.TempDir()
	fakeBin(t, dir, "terraform")
	fakeBin(t, dir, "tofu")
	t.Setenv("PATH", dir)
	got, err := ResolveBinPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "terraform" {
		t.Errorf("expected terraform to win when both exist, got %q", got)
	}
}

func TestResolveBinPath_falls_back_to_tofu(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binaries")
	}
	dir := t.TempDir()
	fakeBin(t, dir, "tofu")
	t.Setenv("PATH", dir)
	got, err := ResolveBinPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "tofu" {
		t.Errorf("expected tofu fallback, got %q", got)
	}
}

func TestResolveBinPath_none_found(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := ResolveBinPath("")
	if err == nil {
		t.Fatal("expected error when neither terraform nor tofu is on PATH")
	}
}

func TestIsOpenTofu(t *testing.T) {
	cases := map[string]bool{
		"tofu":                   true,
		"/usr/local/bin/tofu":    true,
		`tofu.exe`:               true,
		"terraform":              false,
		"/opt/bin/terraform":     false,
		`terraform.exe`:          false,
		`C:\tools\terraform.exe`: false,
	}
	for bin, want := range cases {
		if got := IsOpenTofu(bin); got != want {
			t.Errorf("IsOpenTofu(%q) = %v, want %v", bin, got, want)
		}
	}
}

func TestNew_default_bin_path(t *testing.T) {
	r := New("", "/some/dir")
	if r.binPath != "terraform" {
		t.Errorf("expected default bin path 'terraform', got %q", r.binPath)
	}
}

func TestNew_custom_bin_path(t *testing.T) {
	r := New("/usr/local/bin/terraform", "/some/dir")
	if r.binPath != "/usr/local/bin/terraform" {
		t.Errorf("expected custom bin path, got %q", r.binPath)
	}
}

func TestExitCodeFrom_nil(t *testing.T) {
	if code := exitCodeFrom(nil); code != 0 {
		t.Errorf("expected 0 for nil error, got %d", code)
	}
}

func TestExitCodeFrom_exit_error(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 2")
	err := cmd.Run()
	if code := exitCodeFrom(err); code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
}

func TestExitCodeFrom_other_error(t *testing.T) {
	// a non-ExitError (e.g. binary not found) returns -1
	cmd := exec.Command("this-binary-does-not-exist-xyz")
	err := cmd.Run()
	if code := exitCodeFrom(err); code != -1 {
		t.Errorf("expected -1 for non-ExitError, got %d", code)
	}
}

func TestParseSummaryJSON_changes(t *testing.T) {
	json := `{
		"resource_changes": [
			{"change": {"actions": ["create"]}},
			{"change": {"actions": ["update"]}},
			{"change": {"actions": ["update"]}},
			{"change": {"actions": ["delete"]}}
		]
	}`
	s, _, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Add != 1 {
		t.Errorf("expected Add=1, got %d", s.Add)
	}
	if s.Change != 2 {
		t.Errorf("expected Change=2, got %d", s.Change)
	}
	if s.Destroy != 1 {
		t.Errorf("expected Destroy=1, got %d", s.Destroy)
	}
}

func TestParseSummaryJSON_no_changes(t *testing.T) {
	json := `{"resource_changes": []}`
	s, _, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Add != 0 || s.Change != 0 || s.Destroy != 0 {
		t.Errorf("expected all zeros, got %+v", s)
	}
}

func TestParseSummaryJSON_invalid_json(t *testing.T) {
	_, _, err := ParseSummaryJSON("not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseSummaryJSON_no_op_action(t *testing.T) {
	// "no-op" actions should not count
	json := `{
		"resource_changes": [
			{"change": {"actions": ["no-op"]}}
		]
	}`
	s, _, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Add != 0 || s.Change != 0 || s.Destroy != 0 {
		t.Errorf("no-op should not count, got %+v", s)
	}
}

func TestParseSummaryJSON_excludes_no_op_from_changes(t *testing.T) {
	// regression: real plans include no-op entries for unchanged resources;
	// they must not appear in ResourceChanges or drift detection breaks
	// when all actionable changes are filtered by ignore rules
	json := `{
		"resource_changes": [
			{"address": "aws_instance.unchanged", "change": {"actions": ["no-op"]}},
			{"address": "data.aws_ami.latest", "change": {"actions": ["read"]}},
			{"address": "aws_instance.drifted", "change": {"actions": ["update"]}}
		]
	}`
	s, changes, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("expected 1 actionable change, got %d", len(changes))
	}
	if changes[0].Address != "aws_instance.drifted" {
		t.Errorf("expected aws_instance.drifted, got %s", changes[0].Address)
	}
	if s.Change != 1 {
		t.Errorf("expected Change=1, got %d", s.Change)
	}
}

func TestRun_timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binary")
	}
	dir := t.TempDir()
	slow := filepath.Join(dir, "slow-terraform")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(slow, dir).WithTimeout(200 * time.Millisecond)
	_, runErr := r.run("init")
	if runErr == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(runErr.Error(), "timed out") {
		t.Errorf("expected timeout message, got: %v", runErr)
	}
}

// fakeTerraform writes a script that records its args and exits with the
// requested code for plan, while answering show/show -json plausibly.
func fakeTerraform(t *testing.T, dir string, planExit int) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/sh
echo "$@" >> %s/args.log
case "$1" in
  plan) exit %d ;;
  show)
    case "$*" in
      *-json*) echo '{"resource_changes":[{"address":"null_resource.x","change":{"actions":["update"]}}]}' ;;
      *) echo "~ null_resource.x" ;;
    esac
    exit 0 ;;
  *) exit 0 ;;
esac
`, dir, planExit)
	bin := filepath.Join(dir, "terraform")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func TestInit_passes_backend_config_sorted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binary")
	}
	dir := t.TempDir()
	bin := fakeTerraform(t, dir, 0)

	r := New(bin, dir).WithBackendConfig(map[string]string{
		"key":    "prod/terraform.tfstate",
		"bucket": "my-state-bucket",
	})
	if err := r.Init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args, err := os.ReadFile(filepath.Join(dir, "args.log"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(args)
	want := "-backend-config=bucket=my-state-bucket -backend-config=key=prod/terraform.tfstate"
	if !strings.Contains(got, want) {
		t.Errorf("expected sorted backend-config flags %q in args, got:\n%s", want, got)
	}
}

func TestInit_no_backend_config_flags_when_unset(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binary")
	}
	dir := t.TempDir()
	bin := fakeTerraform(t, dir, 0)

	if err := New(bin, dir).Init(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	args, err := os.ReadFile(filepath.Join(dir, "args.log"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), "-backend-config") {
		t.Errorf("init without backend config must not pass -backend-config, got:\n%s", args)
	}
}

func TestPlanRefreshOnly_passes_flag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binary")
	}
	dir := t.TempDir()
	bin := fakeTerraform(t, dir, 2)

	r := New(bin, dir)
	result, err := r.PlanRefreshOnly("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.HasChanges {
		t.Error("exit 2 must mean changes")
	}

	args, err := os.ReadFile(filepath.Join(dir, "args.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(args), "-refresh-only") {
		t.Errorf("expected -refresh-only in args, got:\n%s", args)
	}
}

func TestPlan_does_not_pass_refresh_only(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-only fake binary")
	}
	dir := t.TempDir()
	bin := fakeTerraform(t, dir, 0)

	r := New(bin, dir)
	result, err := r.Plan("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.HasChanges {
		t.Error("exit 0 must mean no changes")
	}

	args, err := os.ReadFile(filepath.Join(dir, "args.log"))
	if err != nil {
		t.Fatalf("reading args.log: %v", err)
	}
	if strings.Contains(string(args), "-refresh-only") {
		t.Errorf("normal plan must not pass -refresh-only, got:\n%s", args)
	}
}
