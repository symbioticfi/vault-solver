package decision

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/webhook"
)

// RedStone OEV webhook JSON wire contract: big integers are decimal strings,
// and strategy responses reject unknown fields so remote deciders fail closed on schema drift.

type bidInputJSON struct {
	Now             time.Time                    `json:"now"`
	Auction         auctionSnapshotJSON          `json:"auction"`
	Adapter         adapterSnapshotJSON          `json:"adapter"`
	Context         bidContextJSON               `json:"context"`
	PendingAuctions []pendingAuctionSnapshotJSON `json:"pendingAuctions"`
	Exposure        exposureJSON                 `json:"exposure"`
}

type auctionSnapshotJSON struct {
	ID            string             `json:"id"`
	Timestamp     int64              `json:"timestamp"`
	TimeoutMs     int                `json:"timeoutMs"`
	RawPriceCount int                `json:"rawPriceCount"`
	Prices        []auctionPriceJSON `json:"prices"`
}

type auctionPriceJSON struct {
	Oracle common.Address `json:"oracle"`
	Price  string         `json:"price"`
}

type adapterSnapshotJSON struct {
	Address      common.Address           `json:"address"`
	Vault        common.Address           `json:"vault"`
	Loan         common.Address           `json:"loan"`
	LoanDecimals int                      `json:"loanDecimals"`
	Paused       bool                     `json:"paused"`
	FreeAssets   string                   `json:"freeAssets"`
	Withdrawable string                   `json:"withdrawable"`
	Redeemable   []redeemableSnapshotJSON `json:"redeemable"`
	Filler       bool                     `json:"filler"`
}

type redeemableSnapshotJSON struct {
	Asset          common.Address `json:"asset"`
	Decimals       int            `json:"decimals"`
	MaxRate        string         `json:"maxRate"`
	MaxAssets      string         `json:"maxAssets"`
	AcquireBalance string         `json:"acquireBalance"`
}

type bidContextJSON struct {
	ChainID            string                `json:"chainId"`
	Executor           common.Address        `json:"executor"`
	Callback           common.Address        `json:"callback"`
	Signer             common.Address        `json:"signer"`
	ExecutorDeposit    string                `json:"executorDeposit"`
	ExecutorMinDeposit string                `json:"executorMinDeposit"`
	MaxTxGasPrice      string                `json:"maxTxGasPrice"`
	GasPrices          *gasPriceSnapshotJSON `json:"gasPrices"`
	GasLimit           uint64                `json:"gasLimit"`
}

type gasPriceSnapshotJSON struct {
	TokenOutPerNative map[common.Address]string `json:"tokenOutPerNative"`
}

type pendingAuctionSnapshotJSON struct {
	ID        string    `json:"id"`
	SentAt    time.Time `json:"sentAt"`
	Won       bool      `json:"won"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type bidOutputJSON struct {
	Decision      Decision      `json:"decision"`
	Reason        string        `json:"reason,omitempty"`
	BidAmount     *string       `json:"bidAmount,omitempty"`
	OperationData string        `json:"operationData,omitempty"`
	Exposure      *exposureJSON `json:"exposure,omitempty"`
}

type exposureJSON struct {
	BidNative string              `json:"bidNative"`
	GasNative string              `json:"gasNative"`
	Positions []positionClaimJSON `json:"positions"`
}

type positionClaimJSON struct {
	MarketID common.Hash    `json:"marketId"`
	Borrower common.Address `json:"borrower"`
}

func (out BidOutput) MarshalJSON() ([]byte, error) {
	var bidAmount *string
	if out.BidAmount != nil {
		value := out.BidAmount.String()
		bidAmount = &value
	}
	var operationData string
	if len(out.OperationData) > 0 {
		operationData = hexutil.Encode(out.OperationData)
	}
	var exposure *exposureJSON
	if out.Exposure.BidNative != nil || out.Exposure.GasNative != nil || len(out.Exposure.Positions) > 0 {
		value := exposureToJSON(out.Exposure)
		exposure = &value
	}
	return json.Marshal(bidOutputJSON{
		Decision:      out.Decision,
		Reason:        out.Reason,
		BidAmount:     bidAmount,
		OperationData: operationData,
		Exposure:      exposure,
	})
}

func (in BidInput) MarshalJSON() ([]byte, error) {
	prices := make([]auctionPriceJSON, 0, len(in.Auction.Prices))
	for _, p := range in.Auction.Prices {
		prices = append(prices, auctionPriceJSON{Oracle: p.Oracle, Price: webhook.FormatDecimalOrZero(p.Price)})
	}
	pending := make([]pendingAuctionSnapshotJSON, 0, len(in.PendingAuctions))
	for _, a := range in.PendingAuctions {
		pending = append(pending, pendingAuctionSnapshotJSON(a))
	}
	redeemable := make([]redeemableSnapshotJSON, 0, len(in.Adapter.Redeemable))
	for _, r := range in.Adapter.Redeemable {
		redeemable = append(redeemable, redeemableSnapshotJSON{
			Asset:          r.Asset,
			Decimals:       r.Decimals,
			MaxRate:        webhook.FormatDecimalOrZero(r.MaxRate),
			MaxAssets:      webhook.FormatDecimalOrZero(r.MaxAssets),
			AcquireBalance: webhook.FormatDecimalOrZero(r.AcquireBalance),
		})
	}
	return json.Marshal(bidInputJSON{
		Now: in.Now,
		Auction: auctionSnapshotJSON{
			ID:            in.Auction.ID,
			Timestamp:     in.Auction.Timestamp,
			TimeoutMs:     in.Auction.TimeoutMs,
			RawPriceCount: in.Auction.RawPriceCount,
			Prices:        prices,
		},
		Adapter: adapterSnapshotJSON{
			Address:      in.Adapter.Address,
			Vault:        in.Adapter.Vault,
			Loan:         in.Adapter.Loan,
			LoanDecimals: in.Adapter.LoanDecimals,
			Paused:       in.Adapter.Paused,
			FreeAssets:   webhook.FormatDecimalOrZero(in.Adapter.FreeAssets),
			Withdrawable: webhook.FormatDecimalOrZero(in.Adapter.Withdrawable),
			Redeemable:   redeemable,
			Filler:       in.Adapter.Filler,
		},
		Context: bidContextJSON{
			ChainID:            webhook.FormatDecimalOrZero(in.Context.ChainID),
			Executor:           in.Context.Executor,
			Callback:           in.Context.Callback,
			Signer:             in.Context.Signer,
			ExecutorDeposit:    webhook.FormatDecimalOrZero(in.Context.ExecutorDeposit),
			ExecutorMinDeposit: webhook.FormatDecimalOrZero(in.Context.ExecutorMinDeposit),
			MaxTxGasPrice:      webhook.FormatDecimalOrZero(in.Context.MaxTxGasPrice),
			GasPrices:          gasPricesJSON(in),
			GasLimit:           in.Context.GasLimit,
		},
		PendingAuctions: pending,
		Exposure:        exposureToJSON(in.Exposure),
	})
}

func exposureToJSON(exposure Exposure) exposureJSON {
	positions := make([]positionClaimJSON, len(exposure.Positions))
	for index, claim := range exposure.Positions {
		positions[index] = positionClaimJSON(claim)
	}
	return exposureJSON{
		BidNative: webhook.FormatDecimalOrZero(exposure.BidNative),
		GasNative: webhook.FormatDecimalOrZero(exposure.GasNative),
		Positions: positions,
	}
}

func gasPricesJSON(in BidInput) *gasPriceSnapshotJSON {
	if in.Context.GasPrices == nil {
		return nil
	}
	rates := make(map[common.Address]string, 1)
	if rate := in.Context.GasPrices.TokenOutPerNative(in.Adapter.Loan); rate != nil {
		rates[in.Adapter.Loan] = rate.String()
	}
	return &gasPriceSnapshotJSON{TokenOutPerNative: rates}
}

func (out *BidOutput) UnmarshalJSON(data []byte) error {
	var wire bidOutputJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wire); err != nil {
		return err
	}
	bidAmount, err := webhook.ParseOptionalDecimalPointer(wire.BidAmount, "bidAmount")
	if err != nil {
		return err
	}
	var operationData []byte
	if wire.OperationData != "" {
		operationData, err = hexutil.Decode(wire.OperationData)
		if err != nil {
			return errors.Errorf("operationData: invalid hex: %w", err)
		}
	}
	var exposure Exposure
	if wire.Exposure != nil {
		bidNative, parseErr := webhook.ParseOptionalDecimal(wire.Exposure.BidNative, "exposure.bidNative")
		if parseErr != nil {
			return parseErr
		}
		gasNative, parseErr := webhook.ParseOptionalDecimal(wire.Exposure.GasNative, "exposure.gasNative")
		if parseErr != nil {
			return parseErr
		}
		positions := make([]PositionClaim, len(wire.Exposure.Positions))
		for index, claim := range wire.Exposure.Positions {
			positions[index] = PositionClaim(claim)
		}
		exposure = Exposure{BidNative: bidNative, GasNative: gasNative, Positions: positions}
	}
	*out = BidOutput{
		Decision:      wire.Decision,
		Reason:        wire.Reason,
		BidAmount:     bidAmount,
		OperationData: operationData,
		Exposure:      exposure,
	}
	return nil
}
