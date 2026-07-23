package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

const Name = "default"

const (
	bpsDenominator         = 10_000
	rateScaleDigits        = 18
	defaultRangeCount      = 8
	defaultExecutionBuffer = 12 * time.Second
)

var defaultMinAmount = big.NewInt(1)

type Config struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	RangeCount              int    `yaml:"rangeCount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

type Strategy struct {
	cfg Config

	minAmount       *big.Int
	rangeCount      int
	executionBuffer time.Duration
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, NewFromConfig)
}

func NewFromConfig(raw yaml.Node, _ strategies.Deps) (types.Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	return New(cfg)
}

func New(cfg Config) (*Strategy, error) {
	if cfg.PriceBufferBps < 0 || cfg.PriceBufferBps >= bpsDenominator {
		return nil, errors.Errorf("priceBufferBps: must be in [0,%d), got %d", bpsDenominator, cfg.PriceBufferBps)
	}
	if 2*cfg.PriceBufferBps >= bpsDenominator {
		return nil, errors.Errorf("2 * priceBufferBps: must be < %d", bpsDenominator)
	}
	if cfg.InventoryReserveBps < 0 || cfg.InventoryReserveBps >= bpsDenominator {
		return nil, errors.Errorf("inventoryReserveBps: must be in [0,%d), got %d", bpsDenominator, cfg.InventoryReserveBps)
	}
	rangeCount := cfg.RangeCount
	if rangeCount == 0 {
		rangeCount = defaultRangeCount
	}
	if rangeCount < 1 || rangeCount > types.MaxQuoteRanges {
		return nil, errors.Errorf("rangeCount: must be in [1,%d], got %d", types.MaxQuoteRanges, cfg.RangeCount)
	}
	minAmount := new(big.Int).Set(defaultMinAmount)
	if cfg.MinAmount != "" {
		var err error
		minAmount, err = parse.Big(cfg.MinAmount, "minAmount")
		if err != nil {
			return nil, err
		}
		if minAmount.Sign() <= 0 {
			return nil, errors.New("minAmount: must be positive")
		}
	}
	executionBuffer, err := parse.Duration(
		cfg.ExecutionDeadlineBuffer, defaultExecutionBuffer, "executionDeadlineBuffer",
	)
	if err != nil {
		return nil, err
	}
	return &Strategy{
		cfg: cfg, minAmount: minAmount, rangeCount: rangeCount, executionBuffer: executionBuffer,
	}, nil
}

func decodeConfig(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	return solver.DecodeStrict(node, out)
}
