package main

import (
	"github.com/spf13/cobra"

	// Solver implementations self-register via init(); these blank imports are the only references to
	// concrete solvers. Adding another solver is an import here plus a config switch.
	_ "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/rfq"
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
