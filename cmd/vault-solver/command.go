package main

import (
	"github.com/spf13/cobra"
	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/version"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{Use: "vault-solver", Short: "Run configured vault liquidity solvers", SilenceUsage: true}
	var path string
	var debug bool
	run := &cobra.Command{
		Use: "run", Short: "Run the configured solvers", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var override *bool
			if cmd.Flags().Changed("debug") {
				override = &debug
			}
			return app.Run(cmd.Context(), path, override)
		},
	}
	run.Flags().StringVar(&path, "config", "", "path to YAML configuration (required)")
	run.Flags().BoolVar(&debug, "debug", false, "override observability.debug")
	_ = run.MarkFlagRequired("config")
	root.AddCommand(run, &cobra.Command{
		Use: "version", Short: "Print build information", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cmd.OutOrStdout().Write([]byte(version.String() + "\n"))
			return err
		},
	})
	return root
}
