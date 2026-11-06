package threef

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type intent struct {
	Version         int                   `json:"version"`
	ChainID         uint64                `json:"chainId"`
	Account         common.Address        `json:"account"`
	Broker          common.Address        `json:"broker"`
	Executor        common.Address        `json:"executor"`
	Policy          common.Address        `json:"policy"`
	RecipeHash      common.Hash           `json:"recipeHash"`
	Asset           common.Address        `json:"asset"`
	UnderlyingAsset common.Address        `json:"underlyingAsset"`
	Request         common.Address        `json:"request"`
	Offer           threef.CreateOfferDto `json:"offer"`
	OfferID         uint64                `json:"offerId"`
}

func journalKey(m offerMarket) string {
	return "merkle.threef.offer-intent.v1:" + strconv.FormatUint(m.ChainID, 10) + ":" + strings.ToLower(m.Account.Hex())
}

func newIntent(m offerMarket, request common.Address, offer threef.CreateOfferDto) *intent {
	return &intent{Version: 1, ChainID: m.ChainID, Account: m.Account, Broker: common.HexToAddress(offer.Maker), Executor: m.Executor, Policy: m.Policy, RecipeHash: m.RecipeHash, Asset: m.Asset, UnderlyingAsset: m.UnderlyingAsset, Request: request, Offer: offer}
}

func (i *intent) matchesMarket(m offerMarket) bool {
	return i.Policy == m.Policy && i.Executor == m.Executor && i.RecipeHash == m.RecipeHash && i.Asset == m.Asset && i.UnderlyingAsset == m.UnderlyingAsset
}

func (i *intent) validate(m offerMarket, broker common.Address) error {
	if i.Version != 1 || i.ChainID != m.ChainID || i.Account != m.Account || i.Policy == (common.Address{}) || i.Broker == (common.Address{}) || i.Broker != broker || i.Executor == (common.Address{}) || i.RecipeHash == (common.Hash{}) || i.Asset == (common.Address{}) || i.UnderlyingAsset == (common.Address{}) || i.Request == (common.Address{}) || i.Offer.Maker != strings.ToLower(i.Broker.Hex()) || i.Offer.ExecutionMode != nil || i.Offer.UseCallback || i.Offer.Signature != nil || i.Offer.PushAuthorizationSignature != nil || i.Offer.ChainId != nil || i.Offer.AuthorizationDeadline != nil || len(i.Offer.AdditionalProperties) > 0 {
		return errors.New("3F offer journal identity is invalid")
	}
	if _, err := dtoID(i.Offer.AuctionId); err != nil {
		return err
	}
	for _, value := range []string{i.Offer.Amount, i.Offer.ExpectedReturn, i.Offer.Nonce, i.Offer.Expiration} {
		if _, err := uintAmount(value); err != nil {
			return errors.Errorf("invalid 3F offer journal: %w", err)
		}
	}
	return nil
}

// loadIntents retains the existing account journal, including a single offer
// saved before multi-auction support. The next write stores the full inventory.
func (b *offerBot) loadIntents(m offerMarket) ([]*intent, error) {
	var raw json.RawMessage
	found, err := b.Store.Load(journalKey(m), &raw)
	if err != nil || !found {
		return nil, err
	}
	var pending []*intent
	if len(raw) > 0 && raw[0] == '{' {
		var previous intent
		err = json.Unmarshal(raw, &previous)
		pending = []*intent{&previous}
	} else {
		err = json.Unmarshal(raw, &pending)
	}
	return pending, err
}

func (b *offerBot) saveIntents(m offerMarket, pending []*intent) error {
	if len(pending) == 0 {
		return b.Store.Delete(journalKey(m))
	}
	return b.Store.Save(journalKey(m), pending)
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
// reservation; it must not free cash or erase another auction's durable intent.
func (b *offerBot) reconcileIntents(m offerMarket, offers []threef.OfferDto) ([]*intent, error) {
	pending, err := b.loadIntents(m)
	if err != nil {
		return nil, errors.Errorf("load 3F offer journal: %w", err)
	}
	retained := make([]*intent, 0, len(pending))
	changed := false
	seen := make(map[float32]bool, len(pending))
	for _, previous := range pending {
		if previous == nil {
			return nil, errors.New("nil 3F offer intent")
		}
		if err := previous.validate(m, b.Client.Broker()); err != nil {
			return nil, err
		}
		if seen[previous.Offer.AuctionId] {
			return nil, errors.New("duplicate auction in 3F offer journal")
		}
		seen[previous.Offer.AuctionId] = true
		previousID := previous.OfferID
		matched, err := matchIntent(previous, offers)
		if err != nil {
			return nil, err
		}
		changed = changed || previousID != previous.OfferID
		if matched == nil {
			nonce, _ := uintAmount(previous.Offer.Nonce)
			// The API cannot have stored an unconfirmed nonce wider than 64 bits.
			if previous.OfferID == 0 && !nonce.IsUint64() {
				continue
			}
		} else {
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
	if changed || len(pending) != len(retained) {
		if err := b.saveIntents(m, retained); err != nil {
			return nil, errors.Errorf("journal reconciled 3F offers: %w", err)
		}
	}
	return retained, nil
}

func matchIntent(previous *intent, offers []threef.OfferDto) (*threef.OfferDto, error) {
	var matched *threef.OfferDto
	for index := range offers {
		offer := &offers[index]
		if strings.EqualFold(offer.Maker, previous.Account.Hex()) && offer.Nonce == previous.Offer.Nonce && offer.AuctionId == previous.Offer.AuctionId {
			if matched != nil {
				return nil, errors.New("multiple 3F offers match journal nonce")
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
		return nil, errors.New("3F reconciled offer differs from durable intent")
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
		return nil, errors.New("3F reconciled offer terms differ from durable intent")
	}
	previous.OfferID = id
	return matched, nil
}
