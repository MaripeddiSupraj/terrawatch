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

func (r *Runner) Init() error {
	_, err := r.run("init", "-input=false", "-no-color")
	return err
}

func (r *Runner) Plan(varsFile string) (*PlanResult, error) {
	planFile := filepath.Join(r.workingDir, ".terrawatch-plan")
	defer os.Remove(planFile)

	args := []string{"plan", "-out=" + planFile, "-detailed-exitcode", "-no-color", "-input=false"}
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
		showOut, showErr := r.run("show", "-no-color", planFile)
		if showErr != nil {
			return nil, fmt.Errorf("terraform show: %w", showErr)
		}
		summary, summaryErr := r.parseSummary(planFile)
		if summaryErr != nil {
			// non-fatal — best effort
			summary = &Summary{}
		}
		return &PlanResult{HasChanges: true, Output: showOut, Summary: *summary}, nil
	default:
		return nil, fmt.Errorf("terraform plan failed (exit %d): %s", exitCode, out)
	}
}

func (r *Runner) parseSummary(planFile string) (*Summary, error) {
	out, err := r.run("show", "-json", planFile)
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
