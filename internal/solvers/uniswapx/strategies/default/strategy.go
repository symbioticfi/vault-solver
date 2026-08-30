package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

const Name = "default"

const (
	bpsDenominator         = 10_000
	defaultExecutionBuffer = 12 * time.Second
)

type Config struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

type Strategy struct {
	cfg Config

	minAmount       *big.Int
	executionBuffer time.Duration
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	strategies.Register(Name, strategies.Registration{Factory: NewFromConfig, ValidateConfig: ValidateConfig})
}

func ValidateConfig(raw yaml.Node) error {
	_, err := NewFromConfig(raw)
	return err
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	var cfg Config
	if err := parse.DecodeStrict(raw, &cfg); err != nil {
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
	minAmount := big.NewInt(1)
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
		cfg: cfg, minAmount: minAmount, executionBuffer: executionBuffer,
	}, nil
}
