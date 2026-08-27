package main

import (
	"github.com/go-errors/errors"
	"github.com/spf13/cobra"

	appconfig "github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/solver"
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
	requiresTxManager := false
	for _, entry := range cfg.Solvers {
		if err := solver.ValidateConfig(entry.Name, entry.Config); err != nil {
			return errors.Errorf("invalid config %q: %w", path, err)
		}
		requires, err := solver.RequiresTxManager(entry.Name)
		if err != nil {
			return errors.Errorf("invalid config %q: %w", path, err)
		}
		requiresTxManager = requiresTxManager || requires
	}
	if requiresTxManager {
		if err := cfg.ValidateTxManager(); err != nil {
			return errors.Errorf("invalid config %q: %w", path, err)
		}
	}
	return nil
}
