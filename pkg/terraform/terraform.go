package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Planner is implemented by Runner and can be substituted in tests.
type Planner interface {
	Init() error
	Plan(varsFile string) (*PlanResult, error)
}

type Runner struct {
	binPath    string
	workingDir string
}

type PlanResult struct {
	HasChanges bool
	Output     string
	Summary    Summary
}

type Summary struct {
	Add     int
	Change  int
	Destroy int
}

// planJSON is the minimal subset of `terraform show -json` we need
type planJSON struct {
	ResourceChanges []resourceChange `json:"resource_changes"`
}

type resourceChange struct {
	Change struct {
		Actions []string `json:"actions"`
	} `json:"change"`
}

func New(binPath, workingDir string) *Runner {
	if binPath == "" {
		binPath = "terraform"
	}
	return &Runner{binPath: binPath, workingDir: workingDir}
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
	_, err := r.run("init", "-input=false", "-no-color")
	return err
}

func (r *Runner) Plan(varsFile string) (*PlanResult, error) {
	// planName is relative to workingDir — terraform resolves it from its own CWD
	const planName = ".terrawatch-plan"
	planFileAbs := filepath.Join(r.workingDir, planName)
	defer os.Remove(planFileAbs)

	args := []string{"plan", "-out=" + planName, "-detailed-exitcode", "-no-color", "-input=false"}
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
		summary, summaryErr := r.parseSummary(planName)
		if summaryErr != nil {
			// non-fatal — best effort
			summary = &Summary{}
		}
		return &PlanResult{HasChanges: true, Output: showOut, Summary: *summary}, nil
	default:
		return nil, fmt.Errorf("terraform plan failed (exit %d): %s", exitCode, out)
	}
}

func (r *Runner) parseSummary(planName string) (*Summary, error) {
	out, err := r.run("show", "-json", planName)
	if err != nil {
		return nil, err
	}
	return ParseSummaryJSON(out)
}

// ParseSummaryJSON parses a terraform show -json output into a Summary.
func ParseSummaryJSON(jsonStr string) (*Summary, error) {
	var p planJSON
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		return nil, err
	}
	s := &Summary{}
	for _, rc := range p.ResourceChanges {
		for _, a := range rc.Change.Actions {
			switch a {
			case "create":
				s.Add++
			case "update":
				s.Change++
			case "delete":
				s.Destroy++
			}
		}
	}
	return s, nil
}

func (r *Runner) run(args ...string) (string, error) {
	cmd := exec.Command(r.binPath, args...)
	cmd.Dir = r.workingDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
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
