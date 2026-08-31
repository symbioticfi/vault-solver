package main

import (
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"

	appconfig "github.com/symbioticfi/vault-solver/internal/config"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and validate configuration",
	}
	cmd.AddCommand(newConfigValidateCmd())
	return cmd
}

func newConfigValidateCmd() *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration without network access",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateConfigFile(path); err != nil {
				return err
			}
			cmd.Printf("config %s is valid\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "config", "", "path to the YAML config file (required)")
	_ = cmd.MarkFlagRequired("config")
	return cmd
}

func validateConfigFile(path string) error {
	cfg, err := appconfig.Load(path)
	if err != nil {
		return err
	}
	requiresTxManager, err := validateConfiguredSolvers(cfg.Solvers)
	if err != nil {
		return errors.Errorf("invalid config %q: %w", path, err)
	}
	if requiresTxManager {
		if err := cfg.ValidateTxManager(); err != nil {
			return errors.Errorf("invalid config %q: %w", path, err)
		}
	}
	return nil
}

func validateConfiguredSolvers(entries []appconfig.SolverConfig) (bool, error) {
	requiresTxManager := false
	for _, entry := range entries {
		descriptor, err := findSolverDescriptor(entry.Name)
		if err != nil {
			return false, err
		}
		if err := descriptor.validateConfig(entry.Config); err != nil {
			return false, errors.Errorf("solver %q config: %w", entry.Name, err)
		}
		requiresTxManager = requiresTxManager || !descriptor.externallySubmitted
	}
	return requiresTxManager, nil
}
