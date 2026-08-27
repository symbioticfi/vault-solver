package main

import (
	"github.com/spf13/cobra"

	// Solver implementations self-register via init(); these blank imports are the only references to
	// concrete solvers. Adding another solver is an import here plus a config switch.
	_ "github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/lifi"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/rfq"
	_ "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx"
)

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "vault-solver",
		Short: "Run a pluggable solver strategy against Symbiotic vaults",
		Long: "vault-solver monitors configured Symbiotic vaults and runs one or more pluggable\n" +
			"solver integrations against them.",
		SilenceUsage: true, // don't print usage on RunE errors (only on flag/arg misuse)
	}
	root.AddCommand(newConfigCmd(), newRunCmd(), newVersionCmd())
	return root
}
