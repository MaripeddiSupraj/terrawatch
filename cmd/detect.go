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
If drift is detected, a pull request is opened on GitHub with the plan output.

Use --dry-run to print detected drift without opening a PR.`,
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print drift without opening a PR")
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

	for _, ws := range cfg.Stacks {
		stop := out.StackScanning(ws.Name)

		d := detector.New(cfg)
		results, err := d.DetectOne(ws)
		stop()

		if err != nil {
			out.StackError(ws.Name, err)
			errs++
			continue
		}

		if results == nil {
			out.StackClean(ws.Name)
			clean++
		} else {
			out.StackDrift(ws.Name, results.Plan.Summary)
			drifts = append(drifts, *results)
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

	gh, err := reporter.NewGitHub(cfg.GitHub)
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}

	out.PRStart()
	ctx := context.Background()
	for _, drift := range drifts {
		pr, err := gh.CreateDriftPR(ctx, drift)
		if err != nil {
			out.PRError(drift.Stack.Name, err)
			continue
		}
		out.PROpened(drift.Stack.Name, pr.URL)
	}

	fmt.Fprintln(os.Stdout)
	return nil
}
