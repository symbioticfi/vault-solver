package threef

import (
	"context"
	"crypto/rand"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

type offerClient interface {
	mintAPI
	Broker() common.Address
	Create(ctx context.Context, offer threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error)
	Cancel(ctx context.Context, id uint64) (*threef.CancelOfferResponseDto, error)
}

// offerMarket contains authenticated policy terms and cash at one closed canonical block.
// EligibleRequests comes from native policy/underlying attestation, never API metadata alone.
type offerMarket struct {
	ChainID                                              uint64
	Account, Executor, Policy, Asset, UnderlyingAsset    common.Address
	RecipeHash                                           common.Hash
	Budget, MinimumYield1e8, MaxPrincipal, AvailableCash *big.Int
	EligibleRequests                                     map[common.Address]bool
}

// offerBot and its pending intents belong to the single solver Run goroutine.
// It never signs or sends a transaction. Send=false performs no API writes.
type offerBot struct {
	Client  offerClient
	pending []*intent
	Send    bool
	Now     func() time.Time
}

type liveOffer struct {
	dto                    threef.OfferDto
	id, auctionID          uint64
	asset                  common.Address
	amount, expectedReturn *big.Int
}

func (b *offerBot) Tick(ctx context.Context, market offerMarket, auctions []threef.AuctionDto) error {
	if err := validateMarket(market); err != nil {
		return err
	}
	offers, err := b.readOffers(ctx, market.Account)
	if err != nil {
		return err
	}
	_, err = b.tickInventory(ctx, market, auctions, offers, true)
	return err
}

// readOffers authenticates one inventory shared by maintenance and bidding.
func (b *offerBot) readOffers(ctx context.Context, account common.Address) ([]threef.OfferDto, error) {
	if b.Client == nil {
		return nil, errors.New("3F offer client is required")
	}
	if b.Client.Broker() == (common.Address{}) {
		return nil, errors.New("3F offer broker is required")
	}
	if err := b.Client.Identity(ctx, account); err != nil {
		return nil, errors.Errorf("verify 3F default maker: %w", err)
	}
	offers, err := b.Client.Offers(ctx)
	if err != nil {
		return nil, errors.Errorf("list 3F offers: %w", err)
	}
	return offers, nil
}

// tickInventory returns true after an API mutation so the caller discards its
// shared inventory. Maintenance-only passes never create a new commitment.
func (b *offerBot) tickInventory(ctx context.Context, market offerMarket, auctions []threef.AuctionDto, offers []threef.OfferDto, create bool) (bool, error) {
	if err := validateMarket(market); err != nil {
		return false, err
	}
	if len(auctions) > 4096 || len(offers) > 4096 {
		return false, errors.New("3F offer inventory exceeds 4096-row bound")
	}
	byID, err := indexAuctions(auctions)
	if err != nil {
		return false, err
	}
	live, blocked, err := inventory(market.Account, offers, byID)
	if err != nil {
		return false, err
	}
	if !b.Send {
		return false, nil
	}
	pending, err := b.reconcileIntents(offers)
	if err != nil {
		return false, err
	}
	for _, previous := range pending {
		id, _ := dtoID(previous.Offer.AuctionId)
		blocked[id] = true
	}
	reserved, pairReserved := reserveOffers(market, live, pending)
	capacity := minInt(new(big.Int).Sub(market.Budget, pairReserved), new(big.Int).Sub(market.AvailableCash, reserved))
	for _, offer := range live {
		if offer.dto.Status != "SUBMITTED" {
			continue
		}
		request := common.HexToAddress(offer.dto.RequestId)
		previous := findIntent(pending, offer.id)
		owned := previous != nil && previous.Executor == market.Executor && previous.Policy == market.Policy && previous.Asset == market.Asset && previous.UnderlyingAsset == market.UnderlyingAsset
		if !owned && !market.EligibleRequests[request] {
			continue
		}
		cancel := owned && (!previous.matchesMarket(market) || !market.EligibleRequests[request])
		if offer.asset != market.Asset && !cancel {
			continue
		}
		cancel = cancel || (offer.asset == market.Asset && (reserved.Cmp(market.AvailableCash) > 0 || pairReserved.Cmp(market.Budget) > 0))
		if offer.asset == market.Asset && market.EligibleRequests[request] {
			fits, fitErr := fitsTerms(market, offer.amount, offer.expectedReturn, byID[offer.auctionID])
			if fitErr != nil {
				return false, fitErr
			}
			cancel = cancel || !fits
		}
		if cancel {
			response, cancelErr := b.Client.Cancel(ctx, offer.id)
			if cancelErr != nil {
				return false, errors.Errorf("cancel 3F submitted offer: %w", cancelErr)
			}
			if response == nil || response.Status != "CANCELLED" {
				return false, errors.New("3F cancel did not confirm cancellation")
			}
			id, idErr := dtoID(response.Id)
			if idErr != nil || id != offer.id {
				return false, errors.New("3F cancel returned a different offer")
			}
			return true, nil
		}
	}
	for _, previous := range pending {
		if previous.OfferID != 0 || !previous.matchesMarket(market) {
			continue
		}
		auctionID, _ := dtoID(previous.Offer.AuctionId)
		auction, exists := byID[auctionID]
		amount, _ := uintAmount(previous.Offer.Amount)
		yield, _ := uintAmount(previous.Offer.ExpectedReturn)
		// The original amount is already reserved; only other offers reduce
		// the cash and pair budget available to replay this exact request.
		available := new(big.Int).Add(capacity, amount)
		if !exists || !market.EligibleRequests[previous.Request] || amount.Cmp(available) > 0 || amount.Cmp(market.MaxPrincipal) > 0 {
			return false, errors.New("3F unresolved offer cannot be retried within current policy and cash limits")
		}
		fits, err := fitsTerms(market, amount, yield, auction)
		if err != nil {
			return false, err
		}
		if !fits {
			return false, errors.New("3F unresolved offer cannot be retried within current rate limits")
		}
		return true, b.postIntent(ctx, market, previous, false)
	}
	if !create {
		return false, nil
	}
	remaining := capacity
	if remaining.Sign() <= 0 {
		return false, nil
	}
	for _, auction := range auctions {
		if !strings.EqualFold(auction.Status, "open") && !strings.EqualFold(auction.Status, "solvable") {
			continue
		}
		id, _ := dtoID(auction.Id) // indexAuctions already validated every ID.
		request, addressErr := address(auction.RequestId)
		if addressErr != nil {
			return false, addressErr
		}
		if blocked[id] || !market.EligibleRequests[request] {
			continue
		}
		asset, assetErr := depositAsset(auction)
		if assetErr != nil {
			return false, assetErr
		}
		if asset != market.Asset {
			continue
		}
		amountText, ok := auction.GetAmountRequestedOk()
		if !ok || amountText == nil {
			return false, errors.New("3F auction amountRequested is unavailable")
		}
		requested, amountErr := uintAmount(*amountText)
		if amountErr != nil {
			return false, errors.Errorf("3F auction amountRequested: %w", amountErr)
		}
		amount := minInt(remaining, market.MaxPrincipal, requested)
		if amount.Sign() <= 0 {
			continue
		}
		yield := minimumReturn(amount, market.MinimumYield1e8)
		auctionMinimum, _, rateErr := auctionReturnBounds(amount, auction)
		if rateErr != nil {
			return false, rateErr
		}
		if yield.Cmp(auctionMinimum) < 0 {
			yield = auctionMinimum
		}
		fits, fitErr := fitsTerms(market, amount, yield, auction)
		if fitErr != nil {
			return false, fitErr
		}
		if !fits {
			continue
		}
		// 3F stores nonces in 64 bits; positive int64 values fit signed and unsigned storage.
		nonce, randomErr := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if randomErr != nil {
			return false, errors.Errorf("create 3F offer nonce: %w", randomErr)
		}
		nonce.Add(nonce, big.NewInt(1))
		now := time.Now()
		if b.Now != nil {
			now = b.Now()
		}
		if now.Unix() <= 0 || now.Unix() > int64(^uint64(0)>>1)-300 {
			return false, errors.New("3F offer clock is invalid")
		}
		offer := threef.CreateOfferDto{AuctionId: auction.Id, Maker: strings.ToLower(b.Client.Broker().Hex()), Amount: amount.String(), ExpectedReturn: yield.String(), Nonce: nonce.String(), Expiration: strconv.FormatInt(now.Unix()+300, 10)}
		created := newIntent(market, request, offer)
		// Capture the exact request before HTTP can accept it and lose the response.
		b.pending = append(pending, created)
		return true, b.postIntent(ctx, market, created, true)
	}
	return false, nil
}

func (b *offerBot) postIntent(ctx context.Context, market offerMarket, pending *intent, first bool) error {
	response, err := b.Client.Create(ctx, pending.Offer)
	if err != nil {
		// A rejected retry cannot disprove acceptance of the earlier request.
		if first && definiteRejection(err) {
			b.pending = withoutIntent(b.pending, pending)
			return errors.Errorf("3F offer rejected: %w", err)
		}
		return errors.Errorf("3F offer POST outcome unresolved; retained intent for auction %v nonce %s: %w", pending.Offer.AuctionId, pending.Offer.Nonce, err)
	}
	if response == nil {
		return errors.New("3F offer POST outcome unresolved: empty response")
	}
	pending.OfferID, err = dtoID(response.Id)
	if err != nil {
		return err
	}
	observability.Log(ctx).Info("3F PUSH offer submitted", "account", market.Account.Hex(), "auctionId", pending.Offer.AuctionId, "offerId", pending.OfferID)
	return nil
}

// Cash is shared by funding asset; policy budget is shared by funding/underlying
// pair. API rows and their RAM intents represent one reservation, not two.
func reserveOffers(m offerMarket, live []liveOffer, pending []*intent) (cash, pair *big.Int) {
	cash, pair = new(big.Int), new(big.Int)
	listed := make(map[uint64]bool, len(live))
	for _, offer := range live {
		listed[offer.id] = true
		if offer.asset == m.Asset {
			cash.Add(cash, offer.amount)
			previous := findIntent(pending, offer.id)
			ownedPair := previous != nil && previous.Policy == m.Policy && previous.UnderlyingAsset == m.UnderlyingAsset
			if ownedPair || m.EligibleRequests[common.HexToAddress(offer.dto.RequestId)] {
				pair.Add(pair, offer.amount)
			}
		}
	}
	for _, previous := range pending {
		if listed[previous.OfferID] || previous.Asset != m.Asset {
			continue
		}
		amount, _ := uintAmount(previous.Offer.Amount)
		cash.Add(cash, amount)
		if previous.Policy == m.Policy && previous.UnderlyingAsset == m.UnderlyingAsset {
			pair.Add(pair, amount)
		}
	}
	return cash, pair
}

func validateMarket(m offerMarket) error {
	if m.ChainID == 0 || m.Account == (common.Address{}) || m.Executor == (common.Address{}) || m.Policy == (common.Address{}) || m.Asset == (common.Address{}) || m.UnderlyingAsset == (common.Address{}) || m.RecipeHash == (common.Hash{}) {
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
