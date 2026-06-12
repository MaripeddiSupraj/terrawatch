package terraform

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
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
	s, err := ParseSummaryJSON(json)
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
	s, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Add != 0 || s.Change != 0 || s.Destroy != 0 {
		t.Errorf("expected all zeros, got %+v", s)
	}
}

func TestParseSummaryJSON_invalid_json(t *testing.T) {
	_, err := ParseSummaryJSON("not json")
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
	s, err := ParseSummaryJSON(json)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Add != 0 || s.Change != 0 || s.Destroy != 0 {
		t.Errorf("no-op should not count, got %+v", s)
	}
}
