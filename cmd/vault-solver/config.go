package main

import (
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"

	appconfig "github.com/symbioticfi/vault-solver/internal/config"
)

func newConfigCmd() *cobra.Command {
	command := &cobra.Command{Use: "config", Short: "Inspect and validate configuration"}
	command.AddCommand(newConfigValidateCmd())
	return command
}

func newConfigValidateCmd() *cobra.Command {
	var path string
	command := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration without network access",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := validateConfigFile(path); err != nil {
				return err
			}
			command.Printf("config %s is valid\n", path)
			return nil
		},
	}
	command.Flags().StringVar(&path, "config", "", "path to the YAML config file (required)")
	_ = command.MarkFlagRequired("config")
	return command
}

func validateConfigFile(path string) error {
	loaded, err := appconfig.Load(path)
	if err != nil {
		return err
	}
	requiresTransactionLane, err := validateSolverConfigs(loaded.Solvers)
	if err != nil {
		return errors.Errorf("invalid config %q: %w", path, err)
	}
	if requiresTransactionLane {
		if err := loaded.ValidateTxManager(); err != nil {
			return errors.Errorf("invalid config %q: %w", path, err)
		}
	}
	return nil
}
