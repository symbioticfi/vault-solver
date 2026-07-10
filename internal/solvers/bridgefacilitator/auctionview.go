package bridgefacilitator

import (
	"math/big"
	"strconv"
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

// maxRateDeciBps returns the auction's current max rate as an exact count of tenth-basis-points.
// The generated API double is normalized once here; unresolved, negative, non-finite, or more precise
// values fail closed because they cannot safely price an offer.
func (a auctionView) maxRateDeciBps() (*big.Int, bool) {
	r, ok := a.dto.GetMaxRateOk()
	if !ok || r == nil {
		return nil, false
	}
	text := strconv.FormatFloat(*r, 'f', -1, 64)
	rate, ok := new(big.Rat).SetString(text)
	if !ok || rate.Sign() < 0 {
		return nil, false
	}
	rate.Mul(rate, big.NewRat(10, 1))
	if rate.Denom().Cmp(big.NewInt(1)) != 0 {
		return nil, false
	}
	return new(big.Int).Set(rate.Num()), true
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
