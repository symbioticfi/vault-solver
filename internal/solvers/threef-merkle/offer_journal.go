package threefmerkle

import (
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
	return &intent{Version: 1, ChainID: m.ChainID, Account: m.Account, Broker: common.HexToAddress(offer.Maker), Executor: m.Executor, RecipeHash: m.RecipeHash, Asset: m.Asset, UnderlyingAsset: m.UnderlyingAsset, Request: request, Offer: offer}
}

func (i *intent) matchesMarket(m offerMarket) bool {
	return i.Executor == m.Executor && i.RecipeHash == m.RecipeHash && i.Asset == m.Asset && i.UnderlyingAsset == m.UnderlyingAsset
}

func (i *intent) validate(m offerMarket, broker common.Address) error {
	if i.Version != 1 || i.ChainID != m.ChainID || i.Account != m.Account || i.Broker == (common.Address{}) || i.Broker != broker || i.Executor == (common.Address{}) || i.RecipeHash == (common.Hash{}) || i.Asset == (common.Address{}) || i.UnderlyingAsset == (common.Address{}) || i.Request == (common.Address{}) || i.Offer.Maker != strings.ToLower(i.Broker.Hex()) || i.Offer.ExecutionMode != nil || i.Offer.UseCallback || i.Offer.Signature != nil || i.Offer.PushAuthorizationSignature != nil || i.Offer.ChainId != nil || i.Offer.AuthorizationDeadline != nil || len(i.Offer.AdditionalProperties) > 0 {
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

func (b *offerBot) reconcileIntent(m offerMarket, offers []threef.OfferDto) (*intent, error) {
	var previous intent
	found, err := b.Store.Load(journalKey(m), &previous)
	if err != nil {
		return nil, errors.Errorf("load 3F offer journal: %w", err)
	}
	if !found {
		return nil, nil
	}
	if err := previous.validate(m, b.Client.Broker()); err != nil {
		return nil, err
	}
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
	// Expiration cannot prove an accepted authorization has disappeared. An unknown
	// POST remains blocked even after restart until its exact API row is observed.
	if matched == nil {
		return nil, errors.New("3F offer POST outcome unresolved; waiting for maker/nonce/auction reconciliation")
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
	if previous.OfferID == 0 {
		previous.OfferID = id
		if err := b.Store.Save(journalKey(m), previous); err != nil {
			return nil, errors.Errorf("journal reconciled 3F offer: %w", err)
		}
	}
	return &previous, nil
}
