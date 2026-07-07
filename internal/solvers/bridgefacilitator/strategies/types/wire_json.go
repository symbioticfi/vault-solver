package types

import (
	"bytes"
	"encoding/json"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// 3F webhook JSON wire contract: big integers are decimal strings, and strategy responses reject
// unknown fields so remote deciders fail closed on schema drift.
type offerInputJSON struct {
	Now        time.Time             `json:"now"`
	Adapters   []adapterSnapshotJSON `json:"adapters"`
	Auctions   []auctionSnapshotJSON `json:"auctions"`
	LiveOffers []liveOfferJSON       `json:"liveOffers"`
}

type adapterSnapshotJSON struct {
	ID string `json:"id"`

	Adapter    common.Address `json:"adapter"`
	Vault      common.Address `json:"vault"`
	Collateral common.Address `json:"collateral"`

	Fundable      string `json:"fundable"`
	OpenCount     int    `json:"openCount"`
	MaxAssets     string `json:"maxAssets"`
	MinAssets     string `json:"minAssets"`
	MinYieldBps   string `json:"minYieldBps"`
	MaxConcurrent int    `json:"maxConcurrent"`
}

type auctionSnapshotJSON struct {
	ID            string `json:"id"`
	AuctionID     int64  `json:"auctionId"`
	OriginalIndex int    `json:"originalIndex"`

	Request      common.Address `json:"request"`
	Status       string         `json:"status"`
	DepositAsset common.Address `json:"depositAsset"`

	AmountRequested string  `json:"amountRequested"`
	RemainingAmount string  `json:"remainingAmount"`
	MaxRateBps      float64 `json:"maxRateBps"`
}

type liveOfferJSON struct {
	AdapterID string `json:"adapterId"`
	AuctionID int64  `json:"auctionId"`
}

type offerOutputJSON struct {
	Offers []offerExecutionJSON `json:"offers"`
}

type offerExecutionJSON struct {
	AuctionID      int64          `json:"auctionId"`
	Request        common.Address `json:"request"`
	Maker          common.Address `json:"maker"`
	Principal      string         `json:"principal"`
	ExpectedReturn string         `json:"expectedReturn"`
	Reason         string         `json:"reason"`
}

func (in OfferInput) MarshalJSON() ([]byte, error) {
	adapters := make([]adapterSnapshotJSON, 0, len(in.Adapters))
	for _, a := range in.Adapters {
		adapters = append(adapters, adapterSnapshotJSON{
			ID: a.ID, Adapter: a.Adapter, Vault: a.Vault, Collateral: a.Collateral,
			Fundable:      bigString(a.Fundable),
			OpenCount:     a.OpenCount,
			MaxAssets:     bigString(a.MaxAssets),
			MinAssets:     bigString(a.MinAssets),
			MinYieldBps:   bigString(a.MinYieldBps),
			MaxConcurrent: a.MaxConcurrent,
		})
	}
	auctions := make([]auctionSnapshotJSON, 0, len(in.Auctions))
	for _, a := range in.Auctions {
		auctions = append(auctions, auctionSnapshotJSON{
			ID: a.ID, AuctionID: a.AuctionID, OriginalIndex: a.OriginalIndex,
			Request: a.Request, Status: a.Status, DepositAsset: a.DepositAsset,
			AmountRequested: bigString(a.AmountRequested),
			RemainingAmount: bigString(a.RemainingAmount),
			MaxRateBps:      a.MaxRateBps,
		})
	}
	liveOffers := make([]liveOfferJSON, 0, len(in.LiveOffers))
	for _, l := range in.LiveOffers {
		liveOffers = append(liveOffers, liveOfferJSON(l))
	}
	return json.Marshal(offerInputJSON{
		Now: in.Now, Adapters: adapters, Auctions: auctions, LiveOffers: liveOffers,
	})
}

func (out *OfferOutput) UnmarshalJSON(b []byte) error {
	var raw offerOutputJSON
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	offers := make([]OfferExecution, 0, len(raw.Offers))
	for i, o := range raw.Offers {
		principal, err := parseBigString(o.Principal, "offers.principal")
		if err != nil {
			return errors.Errorf("offer %d: %w", i, err)
		}
		expectedReturn, err := parseBigString(o.ExpectedReturn, "offers.expectedReturn")
		if err != nil {
			return errors.Errorf("offer %d: %w", i, err)
		}
		offers = append(offers, OfferExecution{
			AuctionID:      o.AuctionID,
			Request:        o.Request,
			Maker:          o.Maker,
			Principal:      principal,
			ExpectedReturn: expectedReturn,
			Reason:         o.Reason,
		})
	}
	*out = OfferOutput{Offers: offers}
	return nil
}

func bigString(n *big.Int) string {
	if n == nil {
		return ""
	}
	return n.String()
}

func parseBigString(s, field string) (*big.Int, error) {
	if s == "" {
		return nil, nil
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid decimal string %q", field, s)
	}
	return n, nil
}
