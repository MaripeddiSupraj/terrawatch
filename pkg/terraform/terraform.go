package terraform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	return &Runner{binPath: binPath, workingDir: workingDir}
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
		summary, changes, summaryErr := r.parseSummary(planName)
		if summaryErr != nil {
			// non-fatal — best effort
			summary = &Summary{}
			changes = nil
		}
		return &PlanResult{HasChanges: true, Output: showOut, Summary: *summary, ResourceChanges: changes}, nil
	default:
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
func ParseSummaryJSON(jsonStr string) (*Summary, []ResourceChange, error) {
	var p planJSON
	if err := json.Unmarshal([]byte(jsonStr), &p); err != nil {
		return nil, nil, err
	}
	s := &Summary{}
	changes := make([]ResourceChange, len(p.ResourceChanges))
	for i, rc := range p.ResourceChanges {
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
		changes[i] = ResourceChange{
			Address: rc.Address,
			Actions: rc.Change.Actions,
			Before:  rc.Change.Before,
			After:   rc.Change.After,
		}
	}
	return s, changes, nil
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
