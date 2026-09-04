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
	asset, present := a.dto.GetDepositAssetOk()
	if !present || asset == nil {
		return ""
	}
	address, present := asset.GetAddressOk()
	if !present || address == nil {
		return ""
	}
	return *address
}

// isOpen reports whether the auction status permits offers.
func (a auctionView) isOpen() bool {
	switch strings.ToLower(a.dto.Status) {
	case "open", "solvable":
		return true
	default:
		return false
	}
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
	return float64(*r), true
}

// amountRequested returns the requested principal, or nil if the API didn't resolve it.
func (a auctionView) amountRequested() *big.Int {
	s, ok := a.dto.GetAmountRequestedOk()
	if !ok || s == nil {
		return nil
	}
	amount := new(big.Int)
	if _, parsed := amount.SetString(*s, 10); !parsed {
		return nil
	}
	return amount
}
