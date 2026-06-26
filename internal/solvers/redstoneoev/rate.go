package redstoneoev

// rate.go holds loan↔ETH rate resolution and loan/native conversions for profitability gates and bid sizing.

import (
	"math/big"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// composeLoanPerEth derives loanPerEth (loan base units per 1 ETH) from two Chainlink-style oracle
// answers — ethUsd (ETH/USD, ethFeedDec decimals) and loanUsd (loan/USD, loanFeedDec decimals) — scaled
// to the loan token's own decimals:
//
//	loanPerEth = ethUsd × 10^(loanDec + loanFeedDec) / (loanUsd × 10^ethFeedDec)
//
// e.g. ETH=$2500, USDC=$1, both feeds 8-dec, loanDec=6 → 2500e8 × 1e6 × 1e8 / (1e8 × 1e8) = 2500e6.
// Returns nil on any non-positive input so callers fail closed.
func composeLoanPerEth(ethUsd, loanUsd *big.Int, ethFeedDec, loanFeedDec, loanDec int) *big.Int {
	if ethUsd == nil || loanUsd == nil || ethUsd.Sign() <= 0 || loanUsd.Sign() <= 0 {
		return nil
	}
	num := new(big.Int).Mul(ethUsd, chain.Exp10(loanDec+loanFeedDec))
	den := new(big.Int).Mul(loanUsd, chain.Exp10(ethFeedDec))
	rate := new(big.Int).Quo(num, den)
	if rate.Sign() <= 0 {
		return nil
	}
	return rate
}

// hasRateSource reports whether a loan↔ETH rate source is configured. Live bidding requires one because
// gas and bid are native-denominated settlement costs while profit is measured in the loan token.
func (c *Config) hasRateSource() bool {
	return c.LoanEthFeed != nil
}

// loanToNative converts loan-token base units to native token base units at the loanPerEth rate, rounding
// down. It returns 0 when no positive rate is available.
func loanToNative(loan, rate *big.Int) *big.Int {
	if rate == nil || rate.Sign() <= 0 || loan == nil {
		return new(big.Int)
	}
	return morpho.MulDivDown(loan, morpho.Wad, rate)
}

// nativeToLoan converts native token base units to loan-token base units at loanPerEth, rounding up so
// cost floors stay conservative.
func nativeToLoan(native, rate *big.Int) *big.Int {
	if rate == nil || rate.Sign() <= 0 || native == nil {
		return new(big.Int)
	}
	return morpho.MulDivUp(native, rate, morpho.Wad)
}
