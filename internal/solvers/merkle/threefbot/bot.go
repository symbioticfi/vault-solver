// Package threefbot submits unsigned API-key PUSH offers for a pinned policy market.
package threefbot

import (
	"context"
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/state"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/merkle/threefbot", "merkle")

type Client interface {
	Broker() common.Address
	Identity(ctx context.Context, account common.Address) error
	Auctions(ctx context.Context) ([]threef.AuctionDto, error)
	Offers(ctx context.Context) ([]threef.OfferDto, error)
	Create(ctx context.Context, offer threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error)
	Cancel(ctx context.Context, id uint64) (*threef.CancelOfferResponseDto, error)
}

// Market contains authenticated policy terms and cash at one closed canonical block.
// EligibleRequests comes from native policy/underlying attestation, never API metadata alone.
type Market struct {
	ChainID                                              uint64
	Account, Executor, Asset, UnderlyingAsset            common.Address
	RecipeHash                                           common.Hash
	Budget, MinimumYield1e8, MaxPrincipal, AvailableCash *big.Int
	EligibleRequests                                     map[common.Address]bool
}

// Bot serializes ticks; the SDK store must have exclusive process ownership.
// It never signs or sends a transaction. Send=false performs no journal or API writes.
type Bot struct {
	Client Client
	Store  *state.Store
	Send   bool
	Now    func() time.Time
	mu     sync.Mutex
}

type liveOffer struct {
	dto                    threef.OfferDto
	id, auctionID          uint64
	asset                  common.Address
	amount, expectedReturn *big.Int
}

func (b *Bot) Tick(ctx context.Context, market Market) (err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ctx, end := tracer.Start(ctx, "merkle.3f.offers.tick", observability.AttrAdapter.String(market.Account.Hex()))
	defer func() { end(err) }()
	if err := validateMarket(market); err != nil {
		return err
	}
	if b.Client == nil || (b.Send && b.Store == nil) {
		return errors.New("3F offer client and send-mode journal are required")
	}
	if b.Client.Broker() == (common.Address{}) {
		return errors.New("3F offer broker is required")
	}
	if err := b.Client.Identity(ctx, market.Account); err != nil {
		return errors.Errorf("verify 3F default maker: %w", err)
	}
	auctions, err := b.Client.Auctions(ctx)
	if err != nil {
		return errors.Errorf("list 3F auctions: %w", err)
	}
	offers, err := b.Client.Offers(ctx)
	if err != nil {
		return errors.Errorf("list 3F offers: %w", err)
	}
	if len(auctions) > 4096 || len(offers) > 4096 {
		return errors.New("3F offer inventory exceeds 4096-row bound")
	}
	byID, err := indexAuctions(auctions)
	if err != nil {
		return err
	}
	live, blocked, err := inventory(market.Account, offers, byID)
	if err != nil {
		return err
	}
	if !b.Send {
		return nil
	}
	previous, err := b.reconcileIntent(market, offers)
	if err != nil {
		return err
	}
	reserved := new(big.Int)
	for _, offer := range live {
		if offer.asset == market.Asset {
			reserved.Add(reserved, offer.amount)
		}
	}
	capacity := minInt(market.Budget, market.AvailableCash)
	for _, offer := range live {
		if offer.dto.Status != "SUBMITTED" {
			continue
		}
		request := common.HexToAddress(offer.dto.RequestId)
		owned := previous != nil && previous.OfferID == offer.id
		cancel := owned && (!previous.matchesMarket(market) || !market.EligibleRequests[request])
		if offer.asset != market.Asset && !cancel {
			continue
		}
		cancel = cancel || (offer.asset == market.Asset && reserved.Cmp(capacity) > 0)
		if offer.asset == market.Asset && market.EligibleRequests[request] {
			fits, fitErr := fitsTerms(market, offer.amount, offer.expectedReturn, byID[offer.auctionID])
			if fitErr != nil {
				return fitErr
			}
			cancel = cancel || !fits
		}
		if cancel {
			response, cancelErr := b.Client.Cancel(ctx, offer.id)
			if cancelErr != nil {
				return errors.Errorf("cancel 3F submitted offer: %w", cancelErr)
			}
			if response == nil || response.Status != "CANCELLED" {
				return errors.New("3F cancel did not confirm cancellation")
			}
			id, idErr := dtoID(response.Id)
			if idErr != nil || id != offer.id {
				return errors.New("3F cancel returned a different offer")
			}
			return nil
		}
	}
	// Keep the single durable intent bound to every live bot-origin offer.
	// Other account offers still contribute reservations and may mint together.
	if previous != nil {
		for _, offer := range live {
			if offer.id == previous.OfferID {
				return nil
			}
		}
	}
	remaining := new(big.Int).Sub(capacity, reserved)
	if remaining.Sign() <= 0 {
		return nil
	}
	for _, auction := range auctions {
		if !strings.EqualFold(auction.Status, "open") && !strings.EqualFold(auction.Status, "solvable") {
			continue
		}
		id, _ := dtoID(auction.Id) // indexAuctions already validated every ID.
		request, addressErr := address(auction.RequestId)
		if addressErr != nil {
			return addressErr
		}
		if blocked[id] || !market.EligibleRequests[request] {
			continue
		}
		asset, assetErr := depositAsset(auction)
		if assetErr != nil {
			return assetErr
		}
		if asset != market.Asset {
			continue
		}
		amountText, ok := auction.GetAmountRequestedOk()
		if !ok || amountText == nil {
			return errors.New("3F auction amountRequested is unavailable")
		}
		requested, amountErr := uintAmount(*amountText)
		if amountErr != nil {
			return errors.Errorf("3F auction amountRequested: %w", amountErr)
		}
		amount := minInt(remaining, market.MaxPrincipal, requested)
		if amount.Sign() <= 0 {
			continue
		}
		yield := minimumReturn(amount, market.MinimumYield1e8)
		fits, fitErr := fitsTerms(market, amount, yield, auction)
		if fitErr != nil {
			return fitErr
		}
		if !fits {
			continue
		}
		var nonce [32]byte
		if _, randomErr := rand.Read(nonce[:]); randomErr != nil {
			return errors.Errorf("create 3F offer nonce: %w", randomErr)
		}
		nonce[0] |= 1
		now := time.Now()
		if b.Now != nil {
			now = b.Now()
		}
		if now.Unix() <= 0 || now.Unix() > int64(^uint64(0)>>1)-300 {
			return errors.New("3F offer clock is invalid")
		}
		offer := threef.CreateOfferDto{AuctionId: auction.Id, Maker: strings.ToLower(b.Client.Broker().Hex()), Amount: amount.String(), ExpectedReturn: yield.String(), Nonce: new(big.Int).SetBytes(nonce[:]).String(), Expiration: strconv.FormatInt(now.Unix()+300, 10)}
		pending := newIntent(market, request, offer)
		if err := b.Store.Save(journalKey(market), pending); err != nil {
			return errors.Errorf("journal 3F offer before POST: %w", err)
		}
		response, postErr := b.Client.Create(ctx, offer)
		if postErr != nil {
			return errors.Errorf("3F offer POST outcome unresolved; retained intent: %w", postErr)
		}
		if response == nil {
			return errors.New("3F offer POST outcome unresolved: empty response")
		}
		pending.OfferID, err = dtoID(response.Id)
		if err != nil {
			return err
		}
		if err := b.Store.Save(journalKey(market), pending); err != nil {
			return errors.Errorf("journal 3F POST receipt: %w", err)
		}
		observability.Log(ctx).Info("3F PUSH offer submitted", "account", market.Account.Hex(), "auctionId", id, "offerId", pending.OfferID)
		return nil
	}
	return nil
}

func validateMarket(m Market) error {
	if m.ChainID == 0 || m.Account == (common.Address{}) || m.Executor == (common.Address{}) || m.Asset == (common.Address{}) || m.UnderlyingAsset == (common.Address{}) || m.RecipeHash == (common.Hash{}) {
		return errors.New("3F market identity is incomplete")
	}
	for _, amount := range []*big.Int{m.Budget, m.MinimumYield1e8, m.MaxPrincipal, m.AvailableCash} {
		if amount == nil || amount.Sign() < 0 || amount.BitLen() > 256 {
			return errors.New("3F market quantity must be uint256")
		}
	}
	return nil
}

func indexAuctions(auctions []threef.AuctionDto) (map[uint64]threef.AuctionDto, error) {
	indexed := make(map[uint64]threef.AuctionDto, len(auctions))
	for _, auction := range auctions {
		id, err := dtoID(auction.Id)
		if err != nil {
			return nil, err
		}
		if _, exists := indexed[id]; exists {
			return nil, errors.New("duplicate 3F auction ID")
		}
		if _, err := address(auction.RequestId); err != nil {
			return nil, err
		}
		indexed[id] = auction
	}
	return indexed, nil
}

func inventory(maker common.Address, offers []threef.OfferDto, auctions map[uint64]threef.AuctionDto) ([]liveOffer, map[uint64]bool, error) {
	live := make([]liveOffer, 0, len(offers))
	blocked := make(map[uint64]bool)
	seen := make(map[uint64]bool)
	for _, offer := range offers {
		owner, err := address(offer.Maker)
		if err != nil {
			return nil, nil, err
		}
		if owner != maker {
			continue
		}
		id, err := dtoID(offer.Id)
		if err != nil {
			return nil, nil, err
		}
		auctionID, err := dtoID(offer.AuctionId)
		if err != nil {
			return nil, nil, err
		}
		if seen[id] {
			return nil, nil, errors.New("duplicate 3F offer ID")
		}
		seen[id] = true
		isLive, err := liveStatus(offer.Status)
		if err != nil {
			return nil, nil, err
		}
		if offer.Status == "CANCELLED" || offer.Status == "CANCELED" {
			blocked[auctionID] = true
		}
		for _, value := range []string{offer.Amount, offer.ExpectedReturn, offer.Nonce, offer.Expiration, offer.FilledAmount, offer.FilledExpectedReturn, offer.PendingAmount, offer.PendingExpectedReturn} {
			if _, err := uintAmount(value); err != nil {
				return nil, nil, errors.Errorf("3F offer quantity: %w", err)
			}
		}
		if !isLive {
			continue
		}
		if offer.ExecutionMode != "PUSH" {
			return nil, nil, errors.New("3F default maker has a live non-PUSH offer")
		}
		blocked[auctionID] = true
		auction, exists := auctions[auctionID]
		if !exists {
			return nil, nil, errors.New("live 3F offer has no auction funding-asset binding")
		}
		request, err := address(offer.RequestId)
		if err != nil || request != common.HexToAddress(auction.RequestId) {
			return nil, nil, errors.New("3F offer request differs from auction")
		}
		asset, err := depositAsset(auction)
		if err != nil {
			return nil, nil, err
		}
		amount, _ := uintAmount(offer.Amount)
		yield, _ := uintAmount(offer.ExpectedReturn)
		pending, _ := uintAmount(offer.PendingAmount)
		if amount.Sign() <= 0 || pending.Cmp(amount) > 0 {
			return nil, nil, errors.New("3F live offer principal is inconsistent")
		}
		// Broadcast/authorized principal excludes historical successful fills.
		// Before a pending fill appears, reserve the full offer conservatively.
		if pending.Sign() > 0 && (offer.Status == "ACCEPTED" || offer.Status == "AWAITING_MINT") {
			amount = pending
		}
		live = append(live, liveOffer{dto: offer, id: id, auctionID: auctionID, asset: asset, amount: amount, expectedReturn: yield})
	}
	return live, blocked, nil
}

func liveStatus(status string) (bool, error) {
	switch status {
	case "SUBMITTED", "ACCEPTED", "AWAITING_MINT", "PENDING":
		return true, nil
	case "SUCCEEDED", "FAILED", "NOT_ACCEPTED", "CANCELLED", "CANCELED", "EXPIRED", "REPAID":
		return false, nil
	default:
		return false, errors.Errorf("unknown 3F offer status %q", status)
	}
}
