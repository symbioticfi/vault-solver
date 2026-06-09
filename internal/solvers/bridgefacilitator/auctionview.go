package bridgefacilitator

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/threef"
)

// auctionView wraps the generated AuctionDto with safe accessors over its many nullable fields.
type auctionView struct {
	dto threef.AuctionDto
}

// matchesAsset reports whether the auction's deposit asset (the stablecoin lent in the auction)
// equals `want` — the funding vault's collateral. This is the link between a 3F auction and a
// Symbiotic vault/adapter: the auction's `vault` is the 3F position manager, not the Symbiotic
// vault, so assets (not vault addresses) are what pair them. The adapter also enforces this on-chain
// (AssetMismatch), so this is the off-chain pre-filter.
func (a auctionView) matchesAsset(want common.Address) bool {
	addr := a.depositAsset()
	if !common.IsHexAddress(addr) {
		return false
	}
	return common.HexToAddress(addr) == want
}

// depositAsset returns the auction's deposit-asset address string for logging ("" if absent).
func (a auctionView) depositAsset() string {
	da, ok := a.dto.GetDepositAssetOk()
	if !ok || da == nil {
		return ""
	}
	if addr, hasAddr := da.GetAddressOk(); hasAddr && addr != nil {
		return *addr
	}
	return ""
}

// isOpen reports whether the auction status permits offers.
func (a auctionView) isOpen() bool {
	return strings.EqualFold(a.dto.Status, "open") || strings.EqualFold(a.dto.Status, "solvable")
}

// requestAddr returns the Request contract address, or the zero address if malformed.
func (a auctionView) requestAddr() common.Address {
	if !common.IsHexAddress(a.dto.RequestId) {
		return common.Address{}
	}
	return common.HexToAddress(a.dto.RequestId)
}

// maxRate returns the auction's current max rate (basis points) as a float64, or 0 if the API
// didn't resolve it (for logging only).
func (a auctionView) maxRate() float64 {
	r, ok := a.dto.GetMaxRateOk()
	if !ok || r == nil {
		return 0
	}
	return float64(*r)
}

// amountRequested returns the requested principal, or nil if the API didn't resolve it.
func (a auctionView) amountRequested() *big.Int {
	s, ok := a.dto.GetAmountRequestedOk()
	if !ok || s == nil {
		return nil
	}
	n, ok := new(big.Int).SetString(*s, 10)
	if !ok {
		return nil
	}
	return n
}
