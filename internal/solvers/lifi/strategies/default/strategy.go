package defaultstrategy

import (
	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/parse"

	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

const Name = "default"

const (
	bpsDenominator    = 10_000
	rateScaleDigits   = 18
	defaultRangeCount = 8
)

type Config struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	RangeCount              int    `yaml:"rangeCount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

type Strategy struct {
	policy     planning.ExecutionPolicy
	rangeCount int
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(cfg Config) (*Strategy, error) {
	policy, err := (planning.ExecutionConfig{PriceBufferBps: cfg.PriceBufferBps, MinAmount: cfg.MinAmount,
		InventoryReserveBps: cfg.InventoryReserveBps, ExecutionDeadlineBuffer: cfg.ExecutionDeadlineBuffer}).Parse()
	if err != nil {
		return nil, err
	}
	count := parse.OrDefault(cfg.RangeCount, defaultRangeCount)
	if count < 1 || count > types.MaxQuoteRanges {
		return nil, errors.Errorf("rangeCount: must be in [1,%d], got %d", types.MaxQuoteRanges, count)
	}
	return &Strategy{policy: policy, rangeCount: count}, nil
}
