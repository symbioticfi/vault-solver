package threef

import (
	"context"
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
	return i.Executor == m.Executor && i.RecipeHash == m.RecipeHash && i.Asset == m.Asset && i.UnderlyingAsset == m.UnderlyingAsset
}

func (i *intent) validate(m offerMarket, broker common.Address) error {
	if i.Version != 1 || i.ChainID != m.ChainID || i.Account != m.Account || i.Policy == (common.Address{}) || i.Policy != m.Policy || i.Broker == (common.Address{}) || i.Broker != broker || i.Executor == (common.Address{}) || i.RecipeHash == (common.Hash{}) || i.Asset == (common.Address{}) || i.UnderlyingAsset == (common.Address{}) || i.Request == (common.Address{}) || i.Offer.Maker != strings.ToLower(i.Broker.Hex()) || i.Offer.ExecutionMode != nil || i.Offer.UseCallback || i.Offer.Signature != nil || i.Offer.PushAuthorizationSignature != nil || i.Offer.ChainId != nil || i.Offer.AuthorizationDeadline != nil || len(i.Offer.AdditionalProperties) > 0 {
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

func (b *offerBot) reconcileIntent(ctx context.Context, m offerMarket, offers []threef.OfferDto, auctions map[uint64]threef.AuctionDto) (*intent, bool, error) {
	var previous intent
	found, err := b.Store.Load(journalKey(m), &previous)
	if err != nil {
		return nil, false, errors.Errorf("load 3F offer journal: %w", err)
	}
	if !found {
		return nil, false, nil
	}
	if err := previous.validate(m, b.Client.Broker()); err != nil {
		return nil, false, err
	}
	var matched *threef.OfferDto
	for index := range offers {
		offer := &offers[index]
		if strings.EqualFold(offer.Maker, previous.Account.Hex()) && offer.Nonce == previous.Offer.Nonce && offer.AuctionId == previous.Offer.AuctionId {
			if matched != nil {
				return nil, false, errors.New("multiple 3F offers match journal nonce")
			}
			matched = offer
		}
	}
	// Replaying the same tuple updates the same API offer, including after a lost response.
	if matched == nil {
		nonce, _ := uintAmount(previous.Offer.Nonce)
		// 3F cannot store a nonce wider than 64 bits. No API row or receipt confirms this offer.
		if previous.OfferID == 0 && !nonce.IsUint64() {
			if err := b.Store.Delete(journalKey(m)); err != nil {
				return nil, false, errors.Errorf("clear unsupported 3F offer nonce: %w", err)
			}
			return nil, false, nil
		}
		if previous.OfferID != 0 || !previous.matchesMarket(m) {
			return nil, false, errors.New("3F offer POST outcome unresolved; waiting for maker/nonce/auction reconciliation")
		}
		auctionID, _ := dtoID(previous.Offer.AuctionId)
		auction, exists := auctions[auctionID]
		amount, _ := uintAmount(previous.Offer.Amount)
		yield, _ := uintAmount(previous.Offer.ExpectedReturn)
		if !exists || !m.EligibleRequests[previous.Request] || amount.Cmp(m.Budget) > 0 || amount.Cmp(m.AvailableCash) > 0 || amount.Cmp(m.MaxPrincipal) > 0 {
			return nil, false, errors.New("3F unresolved offer cannot be retried within current policy and cash limits")
		}
		fits, err := fitsTerms(m, amount, yield, auction)
		if err != nil {
			return nil, false, err
		}
		if !fits {
			return nil, false, errors.New("3F unresolved offer cannot be retried within current rate limits")
		}
		return &previous, true, b.postIntent(ctx, m, &previous, false)
	}
	id, err := dtoID(matched.Id)
	if err != nil {
		return nil, false, err
	}
	if matched.ExecutionMode != "PUSH" || common.HexToAddress(matched.RequestId) != previous.Request || matched.Expiration != previous.Offer.Expiration || (previous.OfferID != 0 && previous.OfferID != id) {
		return nil, false, errors.New("3F reconciled offer differs from durable intent")
	}
	amount, err := uintAmount(matched.Amount)
	if err != nil {
		return nil, false, err
	}
	yield, err := uintAmount(matched.ExpectedReturn)
	if err != nil {
		return nil, false, err
	}
	originalAmount, _ := uintAmount(previous.Offer.Amount)
	originalYield, _ := uintAmount(previous.Offer.ExpectedReturn)
	if amount.Cmp(originalAmount) > 0 || yield.Cmp(originalYield) > 0 || (matched.Status == "SUBMITTED" && (amount.Cmp(originalAmount) != 0 || yield.Cmp(originalYield) != 0)) {
		return nil, false, errors.New("3F reconciled offer terms differ from durable intent")
	}
	if previous.OfferID == 0 {
		previous.OfferID = id
		if err := b.Store.Save(journalKey(m), previous); err != nil {
			return nil, false, errors.Errorf("journal reconciled 3F offer: %w", err)
		}
	}
	return &previous, false, nil
}
