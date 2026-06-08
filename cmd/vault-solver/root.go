package main

import (
	"github.com/spf13/cobra"

	// Solver implementations self-register via init(); this blank import is the only reference to a
	// concrete solver. Adding another solver is an import here plus a config switch.
	_ "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "vault-solver",
		Short: "Run a pluggable solver strategy against Symbiotic vaults",
		Long: "vault-solver monitors a configured selection of Symbiotic vaults and runs a pluggable\n" +
			"solver strategy against them. The first implementation is the 3F Bridge Facilitator.",
		SilenceUsage: true, // don't print usage on RunE errors (only on flag/arg misuse)
	}
	root.AddCommand(newRunCmd(), newVersionCmd())
	return root
}
