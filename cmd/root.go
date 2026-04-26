package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var (
	buildVersion = "dev"
	buildCommit  = "none"
	buildDate    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "terrawatch",
	Short: "Detect Terraform infrastructure drift and open a pull request",
	Long: `terrawatch watches your Terraform stacks for infrastructure drift.
When drift is detected it automatically opens a pull request so your team
can review and reconcile the change.`,
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, _ []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "terrawatch %s (commit: %s, built: %s)\n", buildVersion, buildCommit, buildDate)
	},
}

func SetVersion(version, commit, date string) {
	buildVersion = version
	buildCommit = commit
	buildDate = date
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "terrawatch.yaml", "config file path")
	rootCmd.AddCommand(versionCmd)
}
