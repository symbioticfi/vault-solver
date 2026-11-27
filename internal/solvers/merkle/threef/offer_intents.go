package threef

import (
	"fmt"
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
				return nil, errors.Errorf("multiple 3F offers on auction %v match intent nonce %s", previous.Offer.AuctionId, previous.Offer.Nonce)
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
	var differs string
	switch {
	case matched.ExecutionMode != "PUSH":
		differs = fmt.Sprintf("execution mode %.32q is not PUSH", matched.ExecutionMode)
	case common.HexToAddress(matched.RequestId) != previous.Request:
		differs = fmt.Sprintf("requestId %.66q differs from intent request %s", matched.RequestId, previous.Request.Hex())
	case matched.Expiration != previous.Offer.Expiration:
		differs = fmt.Sprintf("expiration %.80q differs from intent expiration %s", matched.Expiration, previous.Offer.Expiration)
	case previous.OfferID != 0 && previous.OfferID != id:
		differs = fmt.Sprintf("offer ID differs from intent offer ID %d", previous.OfferID)
	}
	if differs != "" {
		return nil, errors.Errorf("3F reconciled offer %d for auction %v nonce %s: %s", id, previous.Offer.AuctionId, previous.Offer.Nonce, differs)
	}
	amount, err := uintAmount(matched.Amount)
	if err != nil {
		return nil, errors.Errorf("3F reconciled offer %d amount: %w", id, err)
	}
	yield, err := uintAmount(matched.ExpectedReturn)
	if err != nil {
		return nil, errors.Errorf("3F reconciled offer %d expectedReturn: %w", id, err)
	}
	originalAmount, _ := uintAmount(previous.Offer.Amount)
	originalYield, _ := uintAmount(previous.Offer.ExpectedReturn)
	submitted := matched.Status == "SUBMITTED"
	switch {
	case amount.Cmp(originalAmount) > 0 || submitted && amount.Cmp(originalAmount) != 0:
		return nil, errors.Errorf("3F reconciled %s offer %d amount %s differs from intent amount %s", matched.Status, id, amount, originalAmount)
	case yield.Cmp(originalYield) > 0 || submitted && yield.Cmp(originalYield) != 0:
		return nil, errors.Errorf("3F reconciled %s offer %d expectedReturn %s differs from intent expectedReturn %s", matched.Status, id, yield, originalYield)
	}
	previous.OfferID = id
	return matched, nil
}
