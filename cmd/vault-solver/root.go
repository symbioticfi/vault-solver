package main

import "github.com/spf13/cobra"

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
