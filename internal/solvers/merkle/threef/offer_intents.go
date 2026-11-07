package threef

import (
	"slices"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type intent struct {
	ChainID         uint64
	Account         common.Address
	Broker          common.Address
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
	return &intent{ChainID: m.ChainID, Account: m.Account, Broker: common.HexToAddress(offer.Maker), Executor: m.Executor, Policy: m.Policy, RecipeHash: m.RecipeHash, Asset: m.Asset, UnderlyingAsset: m.UnderlyingAsset, Request: request, Offer: offer}
}

func (i *intent) matchesMarket(m offerMarket) bool {
	return i.Policy == m.Policy && i.Executor == m.Executor && i.RecipeHash == m.RecipeHash && i.Asset == m.Asset && i.UnderlyingAsset == m.UnderlyingAsset
}

func (i *intent) validate(m offerMarket, broker common.Address) error {
	if i.ChainID != m.ChainID || i.Account != m.Account || i.Policy == (common.Address{}) || i.Broker == (common.Address{}) || i.Broker != broker || i.Executor == (common.Address{}) || i.RecipeHash == (common.Hash{}) || i.Asset == (common.Address{}) || i.UnderlyingAsset == (common.Address{}) || i.Request == (common.Address{}) || i.Offer.Maker != strings.ToLower(i.Broker.Hex()) || i.Offer.ExecutionMode != nil || i.Offer.UseCallback || i.Offer.Signature != nil || i.Offer.PushAuthorizationSignature != nil || i.Offer.ChainId != nil || i.Offer.AuthorizationDeadline != nil || len(i.Offer.AdditionalProperties) > 0 {
		return errors.New("3F offer intent identity is invalid")
	}
	if _, err := dtoID(i.Offer.AuctionId); err != nil {
		return err
	}
	for _, value := range []string{i.Offer.Amount, i.Offer.ExpectedReturn, i.Offer.Nonce, i.Offer.Expiration} {
		if _, err := uintAmount(value); err != nil {
			return errors.Errorf("invalid 3F offer intent: %w", err)
		}
	}
	return nil
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
func (b *offerBot) reconcileIntents(m offerMarket, offers []threef.OfferDto) ([]*intent, error) {
	pending := b.pending
	retained := make([]*intent, 0, len(pending))
	seen := make(map[float32]bool, len(pending))
	for _, previous := range pending {
		if previous == nil {
			return nil, errors.New("nil 3F offer intent")
		}
		if err := previous.validate(m, b.Client.Broker()); err != nil {
			return nil, err
		}
		if seen[previous.Offer.AuctionId] {
			return nil, errors.New("duplicate auction in 3F offer intent")
		}
		seen[previous.Offer.AuctionId] = true
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
