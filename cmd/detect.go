package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
	"github.com/MaripeddiSupraj/terrawatch/internal/reporter"
	"github.com/MaripeddiSupraj/terrawatch/internal/ui"
)

var dryRun bool

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect drift across all configured stacks",
	Long: `Runs terraform plan on each configured stack.
If drift is detected, a pull request is opened on GitHub or GitLab with the plan output.

Use --dry-run to print detected drift without opening a PR.`,
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print drift without opening a PR/MR")
	rootCmd.AddCommand(detectCmd)
}

func runDetect(_ *cobra.Command, _ []string) error {
	out := ui.New()
	out.Header(buildVersion)

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	out.ScanStart(len(cfg.Stacks))

	var drifts []detector.DriftResult
	errs := 0
	clean := 0

	for _, s := range cfg.Stacks {
		stop := out.StackScanning(s.Name)
		d := detector.New(cfg)
		result, err := d.DetectOne(s)
		stop()

		if err != nil {
			out.StackError(s.Name, err)
			errs++
			continue
		}
		if result == nil {
			out.StackClean(s.Name)
			clean++
		} else {
			out.StackDrift(s.Name, result.Plan.Summary)
			drifts = append(drifts, *result)
		}
	}

	out.Divider()
	out.Summary(len(cfg.Stacks), len(drifts), clean, errs)

	if len(drifts) == 0 {
		out.NoDrift()
		return nil
	}

	if dryRun {
		fmt.Fprintln(os.Stdout)
		return nil
	}

	r, err := buildReporter(cfg)
	if err != nil {
		return err
	}

	out.PRStart()
	ctx := context.Background()
	for _, drift := range drifts {
		pr, err := r.CreateDriftPR(ctx, drift)
		if err != nil {
			out.PRError(drift.Stack.Name, err)
			continue
		}
		out.PROpened(drift.Stack.Name, pr.URL, pr.Existing)
	}

	fmt.Fprintln(os.Stdout)
	return nil
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

// suppress unused import warning when os is only used in dry-run
var _ = os.Stdout
