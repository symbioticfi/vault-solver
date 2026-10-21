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
	return float64(*r), true
}

// rateBasisSeconds returns the auction's rateBasisSeconds: 0 when the API doesn't send the field yet
// (pre-switch responses quote maxRate per term, see types.ExpectedReturn), and ok=false when it is sent
// but not a positive number, so the auction is skipped rather than priced on a guessed basis.
// shortcut: read from AdditionalProperties until the live 3F spec carries the field; once
// `make refresh-3f-client` generates it, switch to the typed getter (TestAuctionViewRateBasisSeconds flags it).
// The 2026-10-22 pre-release spec declares every nullable scalar (maxRate, amountRequested, ...) as
// `type: object`, which generates maps that fail to decode numbers, so do not generate from that one.
func (a auctionView) rateBasisSeconds() (int64, bool) {
	raw, present := a.dto.AdditionalProperties["rateBasisSeconds"]
	if !present {
		return 0, true
	}
	v, ok := raw.(float64)
	if !ok || v <= 0 {
		return 0, false
	}
	return int64(v), true
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
