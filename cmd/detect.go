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
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

var (
	dryRun    bool
	recursive bool
	binPath   string
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

func runDetect(cmd *cobra.Command, args []string) error {
	out := ui.New()
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
			out.StackDrift(s.Name, result.Plan.Summary, result.HiddenChanges)
			drifts = append(drifts, *result)
		}
	}

	out.Divider()
	out.Summary(len(cfg.Stacks), len(drifts), clean, errs)

	if len(drifts) == 0 && errs == 0 {
		out.NoDrift()
		return nil
	}

	if len(drifts) > 0 && !dryRun {
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
				errs++
				continue
			}
			out.PROpened(drift.Stack.Name, pr.URL, pr.Existing)
		}
	}

	fmt.Fprintln(os.Stdout)
	os.Exit(exitCodeFor(len(drifts), errs))
	return nil
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
