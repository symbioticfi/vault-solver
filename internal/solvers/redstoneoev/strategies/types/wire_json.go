package types

import (
	"bytes"
	"encoding/json"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
)

// RedStone OEV webhook JSON wire contract: big integers are decimal strings,
// and strategy responses reject unknown fields so remote deciders fail closed on schema drift.

type bidInputJSON struct {
	Now             time.Time                    `json:"now"`
	Auction         auctionSnapshotJSON          `json:"auction"`
	Adapter         adapterSnapshotJSON          `json:"adapter"`
	Context         bidContextJSON               `json:"context"`
	PendingAuctions []pendingAuctionSnapshotJSON `json:"pendingAuctions"`
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
	ChainID         string         `json:"chainId"`
	Executor        common.Address `json:"executor"`
	Callback        common.Address `json:"callback"`
	Signer          common.Address `json:"signer"`
	ExecutorDeposit string         `json:"executorDeposit"`
	MaxTxGasPrice   string         `json:"maxTxGasPrice"`
	GasLimit        uint64         `json:"gasLimit"`
}

type pendingAuctionSnapshotJSON struct {
	ID        string    `json:"id"`
	SentAt    time.Time `json:"sentAt"`
	Won       bool      `json:"won"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type bidOutputJSON struct {
	Decision      Decision `json:"decision"`
	Reason        string   `json:"reason,omitempty"`
	BidAmount     *string  `json:"bidAmount,omitempty"`
	OperationData string   `json:"operationData,omitempty"`
}

func (in BidInput) MarshalJSON() ([]byte, error) {
	prices := make([]auctionPriceJSON, 0, len(in.Auction.Prices))
	for _, p := range in.Auction.Prices {
		prices = append(prices, auctionPriceJSON{Oracle: p.Oracle, Price: bigStringZero(p.Price)})
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
			MaxRate:        bigStringZero(r.MaxRate),
			MaxAssets:      bigStringZero(r.MaxAssets),
			AcquireBalance: bigStringZero(r.AcquireBalance),
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
			FreeAssets:   bigStringZero(in.Adapter.FreeAssets),
			Withdrawable: bigStringZero(in.Adapter.Withdrawable),
			Redeemable:   redeemable,
			Filler:       in.Adapter.Filler,
		},
		Context: bidContextJSON{
			ChainID:         bigStringZero(in.Context.ChainID),
			Executor:        in.Context.Executor,
			Callback:        in.Context.Callback,
			Signer:          in.Context.Signer,
			ExecutorDeposit: bigStringZero(in.Context.ExecutorDeposit),
			MaxTxGasPrice:   bigStringZero(in.Context.MaxTxGasPrice),
			GasLimit:        in.Context.GasLimit,
		},
		PendingAuctions: pending,
	})
}

func (out *BidOutput) UnmarshalJSON(data []byte) error {
	var wire bidOutputJSON
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&wire); err != nil {
		return err
	}
	bidAmount, err := parseOptionalDecimal(wire.BidAmount, "bidAmount")
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
	*out = BidOutput{
		Decision:      wire.Decision,
		Reason:        wire.Reason,
		BidAmount:     bidAmount,
		OperationData: operationData,
	}
	return nil
}

func parseOptionalDecimal(s *string, field string) (*big.Int, error) {
	if s == nil {
		return nil, nil
	}
	return parseRequiredBigString(*s, field)
}

func bigStringZero(n *big.Int) string {
	if n == nil {
		return "0"
	}
	return n.String()
}

func parseRequiredBigString(s, field string) (*big.Int, error) {
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid decimal string %q", field, s)
	}
	return n, nil
}
