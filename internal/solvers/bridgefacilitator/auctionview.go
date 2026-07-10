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

// maxRateBps returns the auction's current max rate (basis points) and whether the API resolved it.
// It prices every offer and gates the per-adapter return floor, so an unresolved rate means we can't
// bid on the auction at all.
func (a auctionView) maxRateBps() (float64, bool) {
	r, ok := a.dto.GetMaxRateOk()
	if !ok || r == nil {
		return 0, false
	}
	return *r, true
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
