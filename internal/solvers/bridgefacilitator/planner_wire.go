package bridgefacilitator

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/webhook"
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
	MinYieldPpm   string `json:"minYieldPpm"`
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
			Fundable:      webhook.FormatDecimal(a.Fundable),
			OpenCount:     a.OpenCount,
			MaxAssets:     webhook.FormatDecimal(a.MaxAssets),
			MinAssets:     webhook.FormatDecimal(a.MinAssets),
			MinYieldPpm:   webhook.FormatDecimal(a.MinYieldPpm),
			MaxConcurrent: a.MaxConcurrent,
		})
	}
	auctions := make([]auctionSnapshotJSON, 0, len(in.Auctions))
	for _, a := range in.Auctions {
		auctions = append(auctions, auctionSnapshotJSON{
			ID: a.ID, AuctionID: a.AuctionID, OriginalIndex: a.OriginalIndex,
			Request: a.Request, Status: a.Status, DepositAsset: a.DepositAsset,
			AmountRequested: webhook.FormatDecimal(a.AmountRequested),
			RemainingAmount: webhook.FormatDecimal(a.RemainingAmount),
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
		principal, err := webhook.ParseOptionalDecimal(o.Principal, "offers.principal")
		if err != nil {
			return errors.Errorf("offer %d: %w", i, err)
		}
		expectedReturn, err := webhook.ParseOptionalDecimal(o.ExpectedReturn, "offers.expectedReturn")
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
