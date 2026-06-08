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
	if a.dto.DepositAsset == nil || !common.IsHexAddress(a.dto.DepositAsset.Address) {
		return false
	}
	return common.HexToAddress(a.dto.DepositAsset.Address) == want
}

// depositAsset returns the auction's deposit-asset address string for logging ("" if absent).
func (a auctionView) depositAsset() string {
	if a.dto.DepositAsset == nil {
		return ""
	}
	return a.dto.DepositAsset.Address
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

// amountRequested returns the requested principal, or nil if the API didn't resolve it.
func (a auctionView) amountRequested() *big.Int {
	if a.dto.AmountRequested == nil {
		return nil
	}
	n, ok := new(big.Int).SetString(*a.dto.AmountRequested, 10)
	if !ok {
		return nil
	}
	return n
}
