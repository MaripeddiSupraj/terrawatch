package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "terrawatch",
	Short: "Detect Terraform infrastructure drift and open a pull request",
	Long: `terrawatch watches your Terraform workspaces for infrastructure drift.
When drift is detected it automatically opens a pull request so your team
can review and reconcile the change.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "terrawatch.yaml", "config file path")
}
