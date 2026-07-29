package liquidlane

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	FillAmountInput          = "input"
	FillAmountOutput         = "output"
	FillAmountPlannedSurplus = "planned_surplus"
)

// FillMetrics records successful LiquidLane fill amounts without additional chain reads.
type FillMetrics struct {
	amounts *prometheus.CounterVec
}

func NewFillMetrics(reg prometheus.Registerer, solver string) (*FillMetrics, error) {
	if reg == nil {
		return nil, errors.New("liquidlane: metrics registerer is required")
	}
	m := &FillMetrics{amounts: prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "liquidlane_fill_amount_atomic_units_total",
		Help: "Successful LiquidLane fill amounts in token atomic units; planned_surplus is gross and excludes gas.",
	}, []string{"token", "kind"})}
	if err := prometheus.WrapRegistererWith(prometheus.Labels{"solver": solver}, reg).Register(m.amounts); err != nil {
		return nil, errors.Errorf("liquidlane: register fill metrics: %w", err)
	}
	return m, nil
}

func (m *FillMetrics) Observe(
	receipt *types.Receipt,
	tokenIn common.Address,
	amountIn *big.Int,
	tokenOut common.Address,
	amountOut *big.Int,
	plannedSurplus *big.Int,
) {
	if m == nil || receipt == nil || receipt.Status != types.ReceiptStatusSuccessful {
		return
	}
	m.add(tokenIn, FillAmountInput, amountIn)
	m.add(tokenOut, FillAmountOutput, amountOut)
	m.add(tokenOut, FillAmountPlannedSurplus, plannedSurplus)
}

func (m *FillMetrics) add(token common.Address, kind string, amount *big.Int) {
	if token == (common.Address{}) || amount == nil || amount.Sign() <= 0 {
		return
	}
	value, _ := new(big.Float).SetInt(amount).Float64()
	m.amounts.WithLabelValues(strings.ToLower(token.Hex()), kind).Add(value)
}

// PlannedSurplus returns the positive difference between planned routed and required output.
func PlannedSurplus(routed, required *big.Int) *big.Int {
	if routed == nil || required == nil || routed.Cmp(required) <= 0 {
		return new(big.Int)
	}
	return new(big.Int).Sub(routed, required)
}
