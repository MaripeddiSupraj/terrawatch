package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/internal/detector"
	"github.com/MaripeddiSupraj/terrawatch/internal/reporter"
)

var dryRun bool

var detectCmd = &cobra.Command{
	Use:   "detect",
	Short: "Detect drift across all configured workspaces",
	Long: `Runs terraform plan on each configured workspace.
If drift is detected, a pull request is opened on GitHub with the plan output.

Use --dry-run to print detected drift without opening a PR.`,
	RunE: runDetect,
}

func init() {
	detectCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print drift without opening a PR")
	rootCmd.AddCommand(detectCmd)
}

func runDetect(cmd *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	d := detector.New(cfg)

	fmt.Fprintf(cmd.OutOrStdout(), "Scanning %d workspace(s) for drift...\n", len(cfg.Workspaces))

	drifts, err := d.Detect()
	if err != nil {
		return err
	}

	if len(drifts) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No drift detected.")
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Drift detected in %d workspace(s).\n\n", len(drifts))

	if dryRun {
		for _, drift := range drifts {
			s := drift.Plan.Summary
			fmt.Fprintf(cmd.OutOrStdout(), "  workspace: %s  (+%d ~%d -%d)\n",
				drift.Workspace.Name, s.Add, s.Change, s.Destroy)
		}
		return nil
	}

	gh, err := reporter.NewGitHub(cfg.GitHub)
	if err != nil {
		return fmt.Errorf("github client: %w", err)
	}

	ctx := context.Background()
	for _, drift := range drifts {
		pr, err := gh.CreateDriftPR(ctx, drift)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  [error] %s: %v\n", drift.Workspace.Name, err)
			continue
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  PR opened: %s\n", pr.URL)
	}

	return nil
}
