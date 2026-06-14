package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
	"github.com/MaripeddiSupraj/terrawatch/internal/reporter"
	"github.com/MaripeddiSupraj/terrawatch/internal/ui"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

var (
	dryRun       bool
	recursive    bool
	binPath      string
	outputFormat string
	quiet        bool
	classify     bool
	stackFilter  []string
)

var detectCmd = &cobra.Command{
	Use:   "detect [dir...]",
	Short: "Detect Terraform/OpenTofu drift",
	Long: `Runs terraform (or tofu) plan to detect infrastructure drift.

Without arguments, checks the current directory.
Pass one or more directories to check specific stacks.
Use -recursive to walk all subdirectories.

If a config file is found, it is used for full mode (PR creation).
Otherwise terrawatch runs in local mode: drift is printed, no PR is opened.

The binary is auto-detected: terraform first, then tofu (OpenTofu).
Override with --bin or terraform.bin_path in the config file.

With --classify (or drift_mode: strict in the config), an extra
refresh-only plan distinguishes real infrastructure drift from
merged-but-unapplied code changes. In strict mode, unapplied changes
are reported informationally and do not fail the run.

Exit codes (mirrors terraform plan -detailed-exitcode):
  0  no drift detected
  1  error
  2  drift detected`,
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print drift without opening a PR/MR")
	detectCmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "recursively scan subdirectories for Terraform stacks")
	detectCmd.Flags().StringVar(&binPath, "bin", "", "terraform/tofu binary to use (default: auto-detect)")
	detectCmd.Flags().StringVar(&outputFormat, "format", "table", "output format: table or json")
	detectCmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "suppress informational output (errors still print)")
	detectCmd.Flags().BoolVar(&classify, "classify", false, "run a refresh-only plan to tell real infra drift from unapplied code changes")
	detectCmd.Flags().StringSliceVar(&stackFilter, "stack", nil, "only scan the named stack(s) from the config (repeatable)")
	rootCmd.AddCommand(detectCmd)
}

// exitCodeFor mirrors terraform plan -detailed-exitcode:
// errors win over drift so CI can tell an outage from real drift.
func exitCodeFor(drifted, errs int) int {
	switch {
	case errs > 0:
		return 1
	case drifted > 0:
		return 2
	default:
		return 0
	}
}

// stackReport is the per-stack entry of the JSON output. Its shape is part
// of terrawatch's public interface — change it only in a major release.
type stackReport struct {
	Name           string       `json:"name"`
	Path           string       `json:"path"`
	Status         string       `json:"status"` // clean | drift | unapplied | error
	Kind           string       `json:"kind,omitempty"`
	Summary        *jsonSummary `json:"summary,omitempty"`
	IgnoredChanges int          `json:"ignored_changes,omitempty"`
	PRURL          string       `json:"pr_url,omitempty"`
	Error          string       `json:"error,omitempty"`
}

type jsonSummary struct {
	Add     int `json:"add"`
	Change  int `json:"change"`
	Destroy int `json:"destroy"`
}

type runReport struct {
	Version string        `json:"version"`
	Engine  string        `json:"engine"`
	Scanned int           `json:"scanned"`
	Drifted int           `json:"drifted"`
	Clean   int           `json:"clean"`
	Errors  int           `json:"errors"`
	Stacks  []stackReport `json:"stacks"`
}

// filterStacks returns only the named stacks, erroring on unknown names.
func filterStacks(stacks []config.Stack, names []string) ([]config.Stack, error) {
	if len(names) == 0 {
		return stacks, nil
	}
	byName := map[string]config.Stack{}
	for _, s := range stacks {
		byName[s.Name] = s
	}
	var out []config.Stack
	for _, n := range names {
		s, ok := byName[n]
		if !ok {
			return nil, fmt.Errorf("--stack %q does not match any configured stack", n)
		}
		out = append(out, s)
	}
	return out, nil
}

func runDetect(cmd *cobra.Command, args []string) error {
	if outputFormat != "table" && outputFormat != "json" {
		return fmt.Errorf("--format must be table or json, got %q", outputFormat)
	}

	out := ui.New()
	if outputFormat == "json" {
		// keep stdout pure JSON; humans read stderr
		out.SetWriter(os.Stderr)
	}
	out.SetQuiet(quiet)
	out.Header(buildVersion)

	// config file mode — explicit --config or default terrawatch.yaml exists
	configChanged := cmd.Flags().Changed("config")
	cfg, cfgErr := config.Load(cfgFile)

	localMode := false

	if cfgErr != nil && !configChanged {
		// no explicit config — build from args / cwd / recursive walk
		dirs, err := resolveDirs(args)
		if err != nil {
			return err
		}
		cfg = config.LocalConfigFromDirs(dirs)
		localMode = true
		dryRun = true
	} else if cfgErr != nil {
		return fmt.Errorf("load config: %w", cfgErr)
	}

	if localMode {
		out.LocalMode(recursive)
	}

	stacks, err := filterStacks(cfg.Stacks, stackFilter)
	if err != nil {
		return err
	}
	cfg.Stacks = stacks

	// --bin flag wins over config; otherwise auto-detect terraform/tofu
	if binPath != "" {
		cfg.Terraform.BinPath = binPath
	}
	resolved, err := terraform.ResolveBinPath(cfg.Terraform.BinPath)
	if err != nil {
		return err
	}
	cfg.Terraform.BinPath = resolved
	out.Binary(resolved, terraform.IsOpenTofu(resolved))

	out.ScanStart(len(cfg.Stacks))

	det := detector.New(cfg)
	if classify {
		det.Classify = true
	}
	strict := cfg.DriftMode == config.DriftModeStrict

	report := runReport{
		Version: buildVersion,
		Engine:  resolved,
		Scanned: len(cfg.Stacks),
	}

	var drifts []detector.DriftResult
	var cleanStacks []string
	errs := 0

	for _, s := range cfg.Stacks {
		stop := out.StackScanning(s.Name)
		result, err := det.DetectOne(s)
		stop()

		sr := stackReport{Name: s.Name, Path: s.Path}

		switch {
		case err != nil:
			out.StackError(s.Name, err)
			errs++
			sr.Status = "error"
			sr.Error = err.Error()
		case result == nil:
			out.StackClean(s.Name)
			cleanStacks = append(cleanStacks, s.Name)
			sr.Status = "clean"
		case strict && result.Kind == detector.KindUnappliedChanges:
			// the cloud matches state — report informationally, don't fail
			out.StackUnapplied(s.Name, result.Plan.Summary)
			sr.Status = "unapplied"
			sr.Kind = string(result.Kind)
			sr.Summary = summaryJSON(result.Plan.Summary)
		default:
			out.StackDrift(s.Name, result.Plan.Summary, result.HiddenChanges,
				driftKindForUI(result.Kind))
			drifts = append(drifts, *result)
			sr.Status = "drift"
			sr.Kind = string(result.Kind)
			sr.Summary = summaryJSON(result.Plan.Summary)
			sr.IgnoredChanges = result.HiddenChanges
		}

		report.Stacks = append(report.Stacks, sr)
	}

	report.Drifted = len(drifts)
	report.Clean = len(cleanStacks)
	report.Errors = errs

	out.Divider()
	out.Summary(len(cfg.Stacks), len(drifts), len(cleanStacks), errs)

	ctx := context.Background()
	needReporter := !localMode && !dryRun &&
		(len(drifts) > 0 || (cfg.AutoCloseEnabled() && len(cleanStacks) > 0))

	if needReporter {
		r, err := buildReporter(cfg)
		if err != nil {
			return err
		}

		if len(drifts) > 0 {
			out.PRStart()
			for i, drift := range drifts {
				pr, err := r.CreateDriftPR(ctx, drift)
				if err != nil {
					out.PRError(drift.Stack.Name, err)
					errs++
					continue
				}
				out.PROpened(drift.Stack.Name, pr.URL, pr.Existing)
				setPRURL(&report, drifts[i].Stack.Name, pr.URL)
			}
		}

		if cfg.AutoCloseEnabled() {
			for _, name := range cleanStacks {
				pr, err := r.CloseResolvedDriftPR(ctx, name)
				if err != nil {
					// best effort — never fails the run
					out.Warn(name, fmt.Errorf("auto-close: %w", err))
					continue
				}
				if pr != nil {
					out.PRClosed(name, pr.URL)
				}
			}
		}
	}

	report.Errors = errs

	if len(drifts) == 0 && errs == 0 {
		out.NoDrift()
	}

	if outputFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			return fmt.Errorf("encode json: %w", err)
		}
	}

	code := exitCodeFor(len(drifts), errs)
	if code != 0 {
		os.Exit(code)
	}
	return nil
}

func summaryJSON(s terraform.Summary) *jsonSummary {
	return &jsonSummary{Add: s.Add, Change: s.Change, Destroy: s.Destroy}
}

func driftKindForUI(k detector.Kind) ui.DriftKind {
	switch k {
	case detector.KindInfraDrift:
		return ui.DriftInfra
	case detector.KindUnappliedChanges:
		return ui.DriftUnapplied
	default:
		return ui.DriftGeneric
	}
}

func setPRURL(r *runReport, stackName, url string) {
	for i := range r.Stacks {
		if r.Stacks[i].Name == stackName {
			r.Stacks[i].PRURL = url
			return
		}
	}
}

func resolveDirs(args []string) ([]string, error) {
	if recursive {
		root := "."
		if len(args) == 1 {
			root = args[0]
		}
		dirs, err := config.FindTerraformDirs(root)
		if err != nil {
			return nil, fmt.Errorf("scanning directories: %w", err)
		}
		if len(dirs) == 0 {
			return nil, fmt.Errorf("no Terraform files found under %q", root)
		}
		return dirs, nil
	}

	if len(args) > 0 {
		for _, d := range args {
			if !config.HasTerraformFiles(d) {
				return nil, fmt.Errorf("%q contains no Terraform files", d)
			}
		}
		return args, nil
	}

	// default: current directory
	cwd, _ := os.Getwd()
	if !config.HasTerraformFiles(cwd) {
		return nil, fmt.Errorf("no Terraform files in current directory — pass a path or use --config")
	}
	return []string{cwd}, nil
}

func buildReporter(cfg *config.Config) (reporter.Reporter, error) {
	if cfg.GitLab.Repo != "" {
		r, err := reporter.NewGitLab(cfg.GitLab)
		if err != nil {
			return nil, fmt.Errorf("gitlab client: %w", err)
		}
		return r, nil
	}
	r, err := reporter.NewGitHub(cfg.GitHub)
	if err != nil {
		return nil, fmt.Errorf("github client: %w", err)
	}
	return r, nil
}
