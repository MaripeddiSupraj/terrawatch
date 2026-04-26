package terraform

import (
	"os/exec"
	"testing"
)

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
