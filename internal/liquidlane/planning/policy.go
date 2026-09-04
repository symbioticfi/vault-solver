package planning

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

const defaultExecutionLead = 12 * time.Second

// PolicyConfig is the shared greedy quote/fill configuration.
type PolicyConfig struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

// Policy is a validated greedy quote/fill configuration.
type Policy struct {
	PriceBufferBps      int
	InventoryReserveBps int
	MinAmount           *big.Int
	ExecutionBuffer     time.Duration
}

// PolicyFillTask contains current protocol-neutral fill facts plus the protocol's fixed gas envelope.
type PolicyFillTask struct {
	TokenIn, TokenOut  common.Address
	AmountIn           *big.Int
	Quotes             []liquidlane.FillQuote
	Reservations       liquidlane.CapacityReservations
	ValidAfter         time.Time
	MaxRoutes          int
	RequireSingleRoute bool
	InputPolicy        UncoveredInputPolicy
	MaxFeePerGas       *big.Int
	GasPrices          *liquidlanegas.PriceSnapshot
	GasSnapshot        *liquidlanegas.Snapshot
	GasEnvelope        GasEnvelope
	Trace              DecisionTrace
}

// NewPolicy validates and normalizes a PolicyConfig.
func NewPolicy(cfg PolicyConfig) (Policy, error) {
	if cfg.PriceBufferBps < 0 || cfg.PriceBufferBps >= bpsDenominator {
		return Policy{}, errors.Errorf("priceBufferBps: must be in [0,%d), got %d", bpsDenominator, cfg.PriceBufferBps)
	}
	if 2*cfg.PriceBufferBps >= bpsDenominator {
		return Policy{}, errors.Errorf("2 * priceBufferBps: must be < %d", bpsDenominator)
	}
	if cfg.InventoryReserveBps < 0 || cfg.InventoryReserveBps >= bpsDenominator {
		return Policy{}, errors.Errorf(
			"inventoryReserveBps: must be in [0,%d), got %d", bpsDenominator, cfg.InventoryReserveBps,
		)
	}
	minimum := big.NewInt(1)
	if cfg.MinAmount != "" {
		var err error
		minimum, err = parse.Big(cfg.MinAmount, "minAmount")
		if err != nil {
			return Policy{}, err
		}
		if minimum.Sign() <= 0 {
			return Policy{}, errors.New("minAmount: must be positive")
		}
	}
	executionBuffer, err := parse.Duration(cfg.ExecutionDeadlineBuffer, defaultExecutionLead, "executionDeadlineBuffer")
	if err != nil {
		return Policy{}, err
	}
	return Policy{PriceBufferBps: cfg.PriceBufferBps, InventoryReserveBps: cfg.InventoryReserveBps,
		MinAmount: minimum, ExecutionBuffer: executionBuffer}, nil
}

// QuoteCandidates applies reservations and converts inventory into priced candidates.
func (p Policy) QuoteCandidates(
	inventory []liquidlane.Inventory,
	reservations liquidlane.CapacityReservations,
) ([]liquidlane.Inventory, []liquidlane.QuoteCandidate) {
	allocated := AllocateInventoryCapacity(inventory, reservations, p.InventoryReserveBps)
	candidates := make([]liquidlane.QuoteCandidate, 0, len(allocated))
	for _, item := range allocated {
		candidate := NewQuoteCandidate(item, QuoteCapacity(item, p.PriceBufferBps))
		if candidate != nil {
			candidates = append(candidates, *candidate)
		}
	}
	return allocated, candidates
}

// SolveFill applies the shared policy and gas model before entering the greedy allocator.
func (p Policy) SolveFill(task PolicyFillTask) (*FillSolution, error) {
	maxRoutes := task.MaxRoutes
	if task.RequireSingleRoute {
		maxRoutes = 1
	}
	gasPricing, err := NewGasPricing(
		task.MaxFeePerGas, task.TokenOut, task.GasPrices, task.GasSnapshot,
		p.InventoryReserveBps, task.GasEnvelope,
	)
	if err != nil {
		return nil, err
	}
	return SolveFill(FillTask{
		TokenIn: task.TokenIn, TokenOut: task.TokenOut, AmountIn: task.AmountIn,
		Quotes: task.Quotes, Reservations: task.Reservations, ValidAfter: task.ValidAfter,
		MaxRoutes: maxRoutes, PriceBufferBps: p.PriceBufferBps,
		InventoryReserveBps: p.InventoryReserveBps, InputPolicy: task.InputPolicy,
		GasPricing: &gasPricing, Trace: task.Trace,
	})
}
