package main

import (
	"slices"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/bridgefacilitator"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev"
	"github.com/symbioticfi/vault-solver/internal/solvers/rfq"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx"
)

type solverDescriptor struct {
	name                string
	validateConfig      func(yaml.Node) error
	factory             func(yaml.Node, solver.Deps) (solver.Solver, error)
	externallySubmitted bool
}

func solverDescriptors() [5]solverDescriptor {
	return [5]solverDescriptor{
		{
			name:           bridgefacilitator.Name,
			validateConfig: bridgefacilitator.ValidateConfig,
			factory:        bridgefacilitator.Factory,
		},
		{
			name:           lifi.Name,
			validateConfig: lifi.ValidateConfig,
			factory:        lifi.Factory,
		},
		{
			name:                redstoneoev.Name,
			validateConfig:      redstoneoev.ValidateConfig,
			factory:             redstoneoev.Factory,
			externallySubmitted: true,
		},
		{
			name:           rfq.Name,
			validateConfig: rfq.ValidateConfig,
			factory:        rfq.Factory,
		},
		{
			name:           uniswapx.Name,
			validateConfig: uniswapx.ValidateConfig,
			factory:        uniswapx.Factory,
		},
	}
}

func findSolverDescriptor(name string) (solverDescriptor, error) {
	for _, descriptor := range solverDescriptors() {
		if descriptor.name == name {
			return descriptor, nil
		}
	}
	return solverDescriptor{}, errors.Errorf(
		"solver: unknown solver %q (registered: %v)", name, configuredSolverNames(),
	)
}

func configuredSolverNames() []string {
	descriptors := solverDescriptors()
	names := make([]string, len(descriptors))
	for i, descriptor := range descriptors {
		names[i] = descriptor.name
	}
	slices.Sort(names)
	return names
}
