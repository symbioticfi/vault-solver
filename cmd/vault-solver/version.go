package main

import (
	"github.com/spf13/cobra"

	"github.com/symbioticfi/vault-solver/internal/version"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information and exit",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			command.Println(version.String())
		},
	}
}
