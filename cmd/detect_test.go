package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExitCodeFor(t *testing.T) {
	cases := []struct {
		name    string
		drifted int
		errs    int
		want    int
	}{
		{"clean", 0, 0, 0},
		{"drift only", 2, 0, 2},
		{"error only", 0, 1, 1},
		{"errors win over drift", 3, 1, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := exitCodeFor(c.drifted, c.errs); got != c.want {
				t.Errorf("exitCodeFor(%d, %d) = %d, want %d", c.drifted, c.errs, got, c.want)
			}
		})
	}
}

func TestWorkerCount(t *testing.T) {
	cases := []struct {
		name             string
		flag, configured int
		want             int
	}{
		{"defaults to sequential", 0, 0, 1},
		{"config used when no flag", 0, 4, 4},
		{"flag wins over config", 2, 4, 2},
		{"negative flag falls through", -1, 4, 4},
		{"all unset or invalid", -1, 0, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := workerCount(c.flag, c.configured); got != c.want {
				t.Errorf("workerCount(%d, %d) = %d, want %d", c.flag, c.configured, got, c.want)
			}
		})
	}
}

func TestFallBackToLocalMode(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "terrawatch.yaml")
	if err := os.WriteFile(existing, []byte("stacks: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "nope.yaml")

	cases := []struct {
		name          string
		configChanged bool
		path          string
		want          bool
	}{
		{"no flag, file missing → local mode", false, missing, true},
		{"no flag, broken file exists → must error, not fall back", false, existing, false},
		{"explicit --config, file missing → must error", true, missing, false},
		{"explicit --config, file exists → must error", true, existing, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := fallBackToLocalMode(c.configChanged, c.path); got != c.want {
				t.Errorf("fallBackToLocalMode(%v, %q) = %v, want %v", c.configChanged, c.path, got, c.want)
			}
		})
	}
}
