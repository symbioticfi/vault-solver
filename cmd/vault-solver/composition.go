package main

import (
	"slices"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/config"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx"
)

type solverDescriptor struct {
	name                string
	validateConfig      func(yaml.Node) error
	factory             func(yaml.Node, app.Services) (app.Integration, error)
	externallySubmitted bool
}

var solverDescriptors = [...]solverDescriptor{
	{
		name:           bridgefacilitator.Name,
		validateConfig: bridgefacilitator.ValidateConfig,
		factory:        bridgefacilitator.Factory,
	},
	{name: lifi.Name, validateConfig: lifi.ValidateConfig, factory: lifi.Factory},
	{
		name:                redstoneoev.Name,
		validateConfig:      redstoneoev.ValidateConfig,
		factory:             redstoneoev.Factory,
		externallySubmitted: true,
	},
	{name: rfq.Name, validateConfig: rfq.ValidateConfig, factory: rfq.Factory},
	{name: uniswapx.Name, validateConfig: uniswapx.ValidateConfig, factory: uniswapx.Factory},
}

func findSolverDescriptor(name string) (solverDescriptor, error) {
	for _, descriptor := range solverDescriptors {
		if descriptor.name == name {
			return descriptor, nil
		}
	}
	return solverDescriptor{}, errors.Errorf(
		"solver: unknown solver %q (registered: %v)",
		name,
		registeredSolverNames(),
	)
}

func registeredSolverNames() []string {
	names := make([]string, len(solverDescriptors))
	for index, descriptor := range solverDescriptors {
		names[index] = descriptor.name
	}
	slices.Sort(names)
	return names
}

func validateSolverConfigs(entries []config.SolverConfig) (bool, error) {
	requiresTransactionLane := false
	for _, entry := range entries {
		descriptor, err := findSolverDescriptor(entry.Name)
		if err != nil {
			return false, err
		}
		if err := descriptor.validateConfig(entry.Config); err != nil {
			return false, errors.Errorf("solver %q config: %w", entry.Name, err)
		}
		requiresTransactionLane = requiresTransactionLane || !descriptor.externallySubmitted
	}
	return requiresTransactionLane, nil
}

func buildIntegrations(
	entries []config.SolverConfig,
	services app.Services,
) ([]app.Integration, bool, error) {
	integrations := make([]app.Integration, 0, len(entries))
	requiresTransactionLane := false
	for _, entry := range entries {
		descriptor, err := findSolverDescriptor(entry.Name)
		if err != nil {
			return nil, false, err
		}
		integration, err := descriptor.factory(entry.Config, services)
		if err != nil {
			return nil, false, err
		}
		integrations = append(integrations, integration)
		requiresTransactionLane = requiresTransactionLane || !descriptor.externallySubmitted
	}
	return integrations, requiresTransactionLane, nil
}
