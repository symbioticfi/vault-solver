package redstoneoev

import (
	"encoding/hex"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-logr/logr"
)

func logCallbackEvents(log logr.Logger, callback common.Address, expectedAuctionKey common.Hash, receipt *types.Receipt) {
	for _, lg := range receipt.Logs {
		if lg.Address != callback || len(lg.Topics) == 0 {
			continue
		}
		if ev, err := callbackB.UnpackLegResultEvent(lg); err == nil {
			auctionKey := common.BytesToHash(ev.AuctionKey[:])
			if !callbackEventMatchesAuction(log, expectedAuctionKey, auctionKey, "LegResult") {
				continue
			}
			fields := legResultCode(ev.Code)
			log.Info("callback leg result",
				"auctionKey", auctionKey.Hex(),
				"market", common.BytesToHash(ev.MarketId[:]).Hex(),
				"borrower", ev.Borrower.Hex(),
				"index", fields.index, "status", legStatusLabel(fields.status), "reason", legReasonLabel(fields.reason),
				"selector", fields.selector, "seizedAssets", ev.SeizedAssets, "repaidAssets", ev.RepaidAssets,
				"profitLoan", ev.ProfitLoan, "gasUsed", ev.GasUsed)
			continue
		}
		if ev, err := callbackB.UnpackBundleResultEvent(lg); err == nil {
			auctionKey := common.BytesToHash(ev.AuctionKey[:])
			if !callbackEventMatchesAuction(log, expectedAuctionKey, auctionKey, "BundleResult") {
				continue
			}
			log.Info("callback bundle result",
				"auctionKey", auctionKey.Hex(),
				"totalProfitLoan", ev.TotalProfitLoan, "minProfitLoan", ev.MinProfitLoan,
				"gasUsed", ev.GasUsed, "bidAuthorized", ev.BidAuthorized)
			continue
		}
		if ev, err := callbackB.UnpackPayBidResultEvent(lg); err == nil {
			auctionKey := common.BytesToHash(ev.AuctionKey[:])
			if !callbackEventMatchesAuction(log, expectedAuctionKey, auctionKey, "PayBidResult") {
				continue
			}
			log.Info("callback paybid result",
				"auctionKey", auctionKey.Hex(),
				"bidAmount", ev.BidAmount, "paid", ev.Paid)
		}
	}
}

func callbackEventMatchesAuction(log logr.Logger, expected, got common.Hash, event string) bool {
	if expected == (common.Hash{}) || got == expected {
		return true
	}
	log.Info("callback event auction key mismatch", "event", event, "expectedAuctionKey", expected.Hex(), "gotAuctionKey", got.Hex())
	return false
}

type legResultFields struct {
	index    uint64
	status   uint8
	reason   uint8
	selector string
}

func legResultCode(code *big.Int) legResultFields {
	if code == nil {
		return legResultFields{}
	}
	low := code.Uint64()
	out := legResultFields{
		index:  (low >> 16) & 0xffffffffffff,
		status: uint8(low >> 8),
		reason: uint8(low),
	}
	sel := new(big.Int).Rsh(new(big.Int).Set(code), 224)
	if sel.Sign() == 0 {
		return out
	}
	buf := make([]byte, 4)
	sel.FillBytes(buf)
	out.selector = "0x" + hex.EncodeToString(buf)
	return out
}

func legStatusLabel(status uint8) string {
	switch status {
	case 1:
		return "success"
	case 2:
		return "skipped"
	case 3:
		return "reverted"
	default:
		return gasRouteUnknownLabel
	}
}

func legReasonLabel(reason uint8) string {
	switch reason {
	case 0:
		return "none"
	case 1:
		return "swap_output_below_min"
	case 2:
		return "insufficient_loan_proceeds"
	case 3:
		return "profit_below_min"
	case 4:
		return "morpho_revert"
	default:
		return gasRouteUnknownLabel
	}
}
