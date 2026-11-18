package threef

import (
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type intent struct {
	Account         common.Address
	Executor        common.Address
	Policy          common.Address
	RecipeHash      common.Hash
	Asset           common.Address
	UnderlyingAsset common.Address
	Request         common.Address
	Offer           threef.CreateOfferDto
	OfferID         uint64
}

func newIntent(m offerMarket, request common.Address, offer threef.CreateOfferDto) *intent {
	return &intent{Account: m.Account, Executor: m.Executor, Policy: m.Policy, RecipeHash: m.RecipeHash, Asset: m.Asset, UnderlyingAsset: m.UnderlyingAsset, Request: request, Offer: offer}
}

func (i *intent) matchesMarket(m offerMarket) bool {
	return i.Policy == m.Policy && i.Executor == m.Executor && i.RecipeHash == m.RecipeHash && i.Asset == m.Asset && i.UnderlyingAsset == m.UnderlyingAsset
}

func withoutIntent(pending []*intent, removed *intent) []*intent {
	return slices.DeleteFunc(pending, func(i *intent) bool { return i == removed })
}

func findIntent(pending []*intent, id uint64) *intent {
	for _, previous := range pending {
		if previous.OfferID == id {
			return previous
		}
	}
	return nil
}

// Reconcile all auctions before sizing. An absent API row retains its original
// reservation; it must not free cash or erase another auction's RAM intent.
func (b *offerBot) reconcileIntents(offers []threef.OfferDto) ([]*intent, error) {
	pending := b.pending
	retained := make([]*intent, 0, len(pending))
	for _, previous := range pending {
		matched, err := matchIntent(previous, offers)
		if err != nil {
			return nil, err
		}
		if matched != nil {
			live, err := liveStatus(matched.Status)
			if err != nil {
				return nil, err
			}
			if !live {
				continue
			}
		}
		retained = append(retained, previous)
	}
	b.pending = retained
	return retained, nil
}

func matchIntent(previous *intent, offers []threef.OfferDto) (*threef.OfferDto, error) {
	var matched *threef.OfferDto
	for index := range offers {
		offer := &offers[index]
		if strings.EqualFold(offer.Maker, previous.Account.Hex()) && offer.Nonce == previous.Offer.Nonce && offer.AuctionId == previous.Offer.AuctionId {
			if matched != nil {
				return nil, errors.New("multiple 3F offers match intent nonce")
			}
			matched = offer
		}
	}
	if matched == nil {
		return nil, nil
	}
	id, err := dtoID(matched.Id)
	if err != nil {
		return nil, err
	}
	if matched.ExecutionMode != "PUSH" || common.HexToAddress(matched.RequestId) != previous.Request || matched.Expiration != previous.Offer.Expiration || (previous.OfferID != 0 && previous.OfferID != id) {
		return nil, errors.New("3F reconciled offer differs from RAM intent")
	}
	amount, err := uintAmount(matched.Amount)
	if err != nil {
		return nil, err
	}
	yield, err := uintAmount(matched.ExpectedReturn)
	if err != nil {
		return nil, err
	}
	originalAmount, _ := uintAmount(previous.Offer.Amount)
	originalYield, _ := uintAmount(previous.Offer.ExpectedReturn)
	if amount.Cmp(originalAmount) > 0 || yield.Cmp(originalYield) > 0 || (matched.Status == "SUBMITTED" && (amount.Cmp(originalAmount) != 0 || yield.Cmp(originalYield) != 0)) {
		return nil, errors.New("3F reconciled offer terms differ from RAM intent")
	}
	previous.OfferID = id
	return matched, nil
}
