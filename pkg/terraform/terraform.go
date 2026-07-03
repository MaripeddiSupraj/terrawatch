package terraform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultTimeout bounds a single terraform command so a hung plan
// (stuck provider, unreachable backend) cannot stall a CI job forever.
const DefaultTimeout = 30 * time.Minute

// Planner is implemented by Runner and can be substituted in tests.
type Planner interface {
	Init() error
	Plan(varsFile string) (*PlanResult, error)
	// PlanRefreshOnly runs plan -refresh-only: changes mean live
	// infrastructure differs from state (true drift), independent of
	// any unapplied code changes.
	PlanRefreshOnly(varsFile string) (*PlanResult, error)
}

type Runner struct {
	binPath       string
	workingDir    string
	timeout       time.Duration
	backendConfig map[string]string
}

type PlanResult struct {
	HasChanges      bool
	Output          string
	Summary         Summary
	ResourceChanges []ResourceChange
}

type Summary struct {
	Add     int
	Change  int
	Destroy int
}

// ResourceChange is a parsed representation of a single terraform resource change.
type ResourceChange struct {
	Address string
	Actions []string
	Before  map[string]any
	After   map[string]any
}

// planJSON is the minimal subset of `terraform show -json` we need
type planJSON struct {
	ResourceChanges []resourceChange `json:"resource_changes"`
}

type resourceChange struct {
	Address string `json:"address"`
	Change  struct {
		Actions []string       `json:"actions"`
		Before  map[string]any `json:"before"`
		After   map[string]any `json:"after"`
	} `json:"change"`
}

func New(binPath, workingDir string) *Runner {
	if binPath == "" {
		binPath = "terraform"
	}
	return &Runner{binPath: binPath, workingDir: workingDir, timeout: DefaultTimeout}
}

// WithTimeout overrides the per-command timeout. Zero disables it.
func (r *Runner) WithTimeout(d time.Duration) *Runner {
	r.timeout = d
	return r
}

// WithBackendConfig sets key/value overrides passed to init as
// -backend-config=key=value flags (e.g. a per-stack state key).
func (r *Runner) WithBackendConfig(kv map[string]string) *Runner {
	r.backendConfig = kv
	return r
}

// ResolveBinPath returns the IaC binary to use. A configured path wins;
// otherwise the first of terraform/tofu found on PATH is used.
func ResolveBinPath(configured string) (string, error) {
	if configured != "" {
		if _, err := exec.LookPath(configured); err != nil {
			return "", fmt.Errorf("configured binary %q not found: %w", configured, err)
		}
		return configured, nil
	}
	for _, candidate := range []string{"terraform", "tofu"} {
		if _, err := exec.LookPath(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("neither terraform nor tofu (OpenTofu) found on PATH — install one or set terraform.bin_path")
}

// IsOpenTofu reports whether the binary is OpenTofu rather than Terraform.
func IsOpenTofu(binPath string) bool {
	base := filepath.Base(binPath)
	return strings.TrimSuffix(base, filepath.Ext(base)) == "tofu"
}

func (r *Runner) Init() error {
	args := []string{"init", "-input=false", "-no-color"}
	// sorted for a deterministic command line
	keys := make([]string, 0, len(r.backendConfig))
	for k := range r.backendConfig {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, fmt.Sprintf("-backend-config=%s=%s", k, r.backendConfig[k]))
	}
	_, err := r.run(args...)
	return err
}

func (r *Runner) Plan(varsFile string) (*PlanResult, error) {
	return r.plan(varsFile, false)
}

func (r *Runner) PlanRefreshOnly(varsFile string) (*PlanResult, error) {
	return r.plan(varsFile, true)
}

func (r *Runner) plan(varsFile string, refreshOnly bool) (*PlanResult, error) {
	// planName is relative to workingDir — terraform resolves it from its own CWD
	planName := ".terrawatch-plan"
	if refreshOnly {
		planName = ".terrawatch-refresh-plan"
	}
	planFileAbs := filepath.Join(r.workingDir, planName)
	defer os.Remove(planFileAbs)

	args := []string{"plan", "-out=" + planName, "-detailed-exitcode", "-no-color", "-input=false"}
	if refreshOnly {
		args = append(args, "-refresh-only")
	}
	if varsFile != "" {
		args = append(args, "-var-file="+varsFile)
	}

	out, err := r.run(args...)
	exitCode := exitCodeFrom(err)
	switch exitCode {
	case 0:
		return &PlanResult{HasChanges: false, Output: out}, nil
	case 2:
		// changes present — get human-readable output
		showOut, showErr := r.run("show", "-no-color", planName)
		if showErr != nil {
			return nil, fmt.Errorf("terraform show: %w", showErr)
		}
		summary, changes, summaryErr := r.parseSummary(planName)
		if summaryErr != nil {
			// non-fatal — best effort
			summary = &Summary{}
			changes = nil
		}
		return &PlanResult{HasChanges: true, Output: showOut, Summary: *summary, ResourceChanges: changes}, nil
	default:
		if exitCode == -1 && err != nil {
			// not an ExitError — e.g. binary missing or command timeout
			return nil, fmt.Errorf("terraform plan failed: %w", err)
		}
		return nil, fmt.Errorf("terraform plan failed (exit %d): %s", exitCode, out)
	}
}

func (r *Runner) parseSummary(planName string) (*Summary, []ResourceChange, error) {
	out, err := r.run("show", "-json", planName)
	if err != nil {
		return nil, nil, err
	}
	return ParseSummaryJSON(out)
}

// ParseSummaryJSON parses terraform show -json output into a Summary and ResourceChanges.
// The plan JSON includes "no-op" and "read" entries for unchanged resources;
// those are excluded so ResourceChanges only contains actionable changes.
func ParseSummaryJSON(jsonStr string) (*Summary, []ResourceChange, error) {
	var p planJSON
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		return nil, nil, err
	}
	s := &Summary{}
	var changes []ResourceChange
	for _, rc := range p.ResourceChanges {
		actionable := false
		for _, a := range rc.Change.Actions {
			switch a {
			case "create":
				s.Add++
				actionable = true
			case "update":
				s.Change++
				actionable = true
			case "delete":
				s.Destroy++
				actionable = true
			}
		}
		if !actionable {
			continue
		}
		changes = append(changes, ResourceChange{
			Address: rc.Address,
			Actions: rc.Change.Actions,
			Before:  rc.Change.Before,
			After:   rc.Change.After,
		})
	}
	return s, changes, nil
}

func (r *Runner) run(args ...string) (string, error) {
	ctx := context.Background()
	cancel := func() {}
	if r.timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
	}
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binPath, args...)
	cmd.Dir = r.workingDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return buf.String(), fmt.Errorf("%s %s timed out after %s — raise terraform.timeout in the config if plans legitimately take longer",
			filepath.Base(r.binPath), args[0], r.timeout)
	}
	return buf.String(), err
}

func exitCodeFrom(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}
