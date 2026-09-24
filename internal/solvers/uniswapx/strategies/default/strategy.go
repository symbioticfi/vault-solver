package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/single"
	"github.com/symbioticfi/vault-solver/internal/parse"
	"github.com/symbioticfi/vault-solver/internal/solver"
	registry "github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies"
	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

const (
	bpsDenominator         = 10_000
	defaultExecutionBuffer = 12 * time.Second
)

var defaultMinAmount = big.NewInt(1)

type Config struct {
	Name                    string `yaml:"-"`
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

type Strategy struct {
	solveQuote  func(strategies.QuoteTask) (*strategies.QuoteSolution, error)
	solveFill   func(strategies.FillTask) (*strategies.FillSolution, error)
	fillPricing func(types.FillInput) (strategies.GasPricing, error)
	cfg         Config

	minAmount       *big.Int
	executionBuffer time.Duration
}

//nolint:gochecknoinits // solver-local strategy self-registration mirrors solver registration.
func init() {
	registry.Register(types.DefaultName, NewFromConfig)
}

func NewFromConfig(raw yaml.Node) (types.Strategy, error) {
	return newFromConfig(raw, types.DefaultName)
}

// NewSingleFromConfig shares pricing and validation with default, changing only source selection.
func NewSingleFromConfig(raw yaml.Node) (types.Strategy, error) {
	return newFromConfig(raw, types.SingleName)
}

func newFromConfig(raw yaml.Node, name string) (*Strategy, error) {
	var cfg Config
	if err := decodeConfig(raw, &cfg); err != nil {
		return nil, err
	}
	cfg.Name = name
	return New(cfg)
}

func New(cfg Config) (*Strategy, error) {
	if cfg.PriceBufferBps < 0 || cfg.PriceBufferBps >= bpsDenominator {
		return nil, errors.Errorf("priceBufferBps: must be in [0,%d), got %d", bpsDenominator, cfg.PriceBufferBps)
	}
	if cfg.InventoryReserveBps < 0 || cfg.InventoryReserveBps >= bpsDenominator {
		return nil, errors.Errorf("inventoryReserveBps: must be in [0,%d), got %d", bpsDenominator, cfg.InventoryReserveBps)
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
	strategy := &Strategy{
		cfg: cfg, minAmount: minAmount, executionBuffer: executionBuffer,
	}
	switch cfg.Name {
	case "", types.DefaultName:
		strategy.solveQuote, strategy.solveFill = greedy.SolveQuote, greedy.SolveFill
		strategy.fillPricing = strategy.priceFillGas
	case types.SingleName:
		strategy.solveQuote, strategy.solveFill = single.SolveQuote, single.SolveFill
		// The sender pays gas; an awarded single fill needs only the promised output.
		strategy.fillPricing = func(types.FillInput) (strategies.GasPricing, error) {
			return strategies.GasPricing{}, nil
		}
	default:
		return nil, errors.Errorf("UniswapX local strategy %q is not supported", cfg.Name)
	}
	return strategy, nil
}

func decodeConfig(node yaml.Node, out any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	return solver.DecodeStrict(node, out)
}
