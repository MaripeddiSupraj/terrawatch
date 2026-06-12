package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/MaripeddiSupraj/terrawatch/internal/config"
	"github.com/MaripeddiSupraj/terrawatch/pkg/terraform"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the config file and environment before a real run",
	Long: `Checks that terrawatch is ready to run:

  - the config file parses and is well-formed
  - every stack path exists and contains Terraform files
  - the terraform/tofu binary resolves
  - the GitHub/GitLab token authenticates against the configured repo

Exits 1 if any check fails — wire it into CI before the detect job.`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

type check struct {
	name string
	err  error
}

func runValidate(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Printf("  ✗ config: %v\n", err)
		os.Exit(1)
	}

	var checks []check
	checks = append(checks, check{fmt.Sprintf("config %s parses", cfgFile), nil})

	for _, s := range cfg.Stacks {
		var err error
		if _, statErr := os.Stat(s.Path); statErr != nil {
			err = fmt.Errorf("path does not exist: %s", s.Path)
		} else if !config.HasTerraformFiles(s.Path) {
			err = fmt.Errorf("no .tf files in %s", s.Path)
		}
		checks = append(checks, check{fmt.Sprintf("stack %q path", s.Name), err})
	}

	bin, err := terraform.ResolveBinPath(cfg.Terraform.BinPath)
	name := "terraform/tofu binary"
	if err == nil {
		name = fmt.Sprintf("binary resolves (%s)", bin)
	}
	checks = append(checks, check{name, err})

	platform := "github"
	if cfg.GitLab.Repo != "" {
		platform = "gitlab"
	}
	r, err := buildReporter(cfg)
	if err != nil {
		checks = append(checks, check{platform + " client", err})
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		checks = append(checks, check{platform + " token authenticates", r.ValidateAuth(ctx)})
	}

	failed := 0
	fmt.Println()
	for _, c := range checks {
		if c.err != nil {
			fmt.Printf("  ✗ %s: %v\n", c.name, c.err)
			failed++
		} else {
			fmt.Printf("  ✓ %s\n", c.name)
		}
	}
	fmt.Println()

	if failed > 0 {
		fmt.Printf("  %d check(s) failed\n\n", failed)
		os.Exit(1)
	}
	fmt.Println("  All checks passed — terrawatch is ready.")
	fmt.Println()
	return nil
}
