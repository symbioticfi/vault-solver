package lifi

import (
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const defaultPlannerName = "default"

const (
	bpsDenominator    = 10_000
	rateScaleDigits   = 18
	defaultRangeCount = 8
)

type defaultPlannerConfig struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	RangeCount              int    `yaml:"rangeCount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

type defaultPlanner struct {
	policy     liquidplanning.Policy
	rangeCount int
}

func validateDefaultPlannerConfig(raw yaml.Node) error {
	_, err := newDefaultPlannerFromConfig(raw)
	return err
}

func newDefaultPlannerFromConfig(raw yaml.Node) (Planner, error) {
	var cfg defaultPlannerConfig
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return newDefaultPlanner(cfg)
}

func newDefaultPlanner(cfg defaultPlannerConfig) (*defaultPlanner, error) {
	policy, err := liquidplanning.NewPolicy(liquidplanning.PolicyConfig{
		PriceBufferBps:          cfg.PriceBufferBps,
		MinAmount:               cfg.MinAmount,
		InventoryReserveBps:     cfg.InventoryReserveBps,
		ExecutionDeadlineBuffer: cfg.ExecutionDeadlineBuffer,
	})
	if err != nil {
		return nil, err
	}
	rangeCount := cfg.RangeCount
	if rangeCount == 0 {
		rangeCount = defaultRangeCount
	}
	if rangeCount < 1 || rangeCount > MaxQuoteRanges {
		return nil, errors.Errorf("rangeCount: must be in [1,%d], got %d", MaxQuoteRanges, cfg.RangeCount)
	}
	return &defaultPlanner{policy: policy, rangeCount: rangeCount}, nil
}
