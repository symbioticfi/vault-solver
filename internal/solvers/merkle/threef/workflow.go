package threef

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/merkle/threef", Name)

type mintPlanning struct {
	*keeper.Planner

	runtime *mintProvider
}

func (p *mintPlanning) PrepareExecution(ctx context.Context, strategies []keeper.Strategy) (keeper.ExecutionPlan, error) {
	plan, err := p.Planner.PrepareExecution(ctx, strategies)
	if err != nil {
		return plan, err
	}
	for _, member := range plan.Members {
		if len(member.Plan.Outputs) != 1 || len(member.Plan.Outputs[0]) != 1 {
			providers := 0
			if len(member.Plan.Outputs) > 0 {
				providers = len(member.Plan.Outputs[0])
			}
			return keeper.ExecutionPlan{}, errors.Errorf("3f plan has %d output groups and %d providers in the first, want exactly 1 of each", len(member.Plan.Outputs), providers)
		}
		if err := p.runtime.ValidateLease(ctx, member.Plan.Snapshot, member.Plan.Outputs[0][0]); err != nil {
			return keeper.ExecutionPlan{}, err
		}
	}
	return plan, nil
}

type offerWorkflow struct {
	planner  *keeper.Planner
	strategy keeper.Strategy
	policy   common.Address
	api      interface {
		Auctions(context.Context) ([]threef.AuctionDto, error)
	}
	bot *offerBot
}

func (w *offerWorkflow) Tick(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "threef-merkle.offers.tick", observability.AttrAdapter.String(w.strategy.Config.Account))
	defer func() {
		// Wrapped read and SDK errors keep their RPC errors; name any revert once, here.
		err = reverts.Explain(err)
		end(err)
	}()
	p := w.planner
	prepared, err := recipes.DiscoverBatch(ctx, p.Chain, []common.Address{common.HexToAddress(w.strategy.Config.Executor)}, []common.Address{common.HexToAddress(w.strategy.Config.Account)}, p.Caller, recipes.PreparationOptions{ChainID: p.ChainID, Cache: p.RecipeCache})
	if err != nil {
		return err
	}
	snapshot, recipe := prepared[0].Snapshot, prepared[0].Recipe
	pin := provider.Snapshot{ChainID: new(big.Int).Set(p.ChainID), BlockNumber: new(big.Int).SetUint64(snapshot.BlockNumber), BlockHash: snapshot.BlockHash, Timestamp: snapshot.Timestamp, Executor: snapshot.Executor, Account: snapshot.Account, Caller: snapshot.Caller, RecipeHash: snapshot.RecipeHash, Root: snapshot.Root}
	if err := verifyPolicyAccount(ctx, p.Chain, w.policy, pin.Account, pin.BlockNumber); err != nil {
		return err
	}
	auctions, err := w.api.Auctions(ctx)
	if err != nil {
		return err
	}
	if len(auctions) > maxRows {
		return errors.Errorf("3f auction inventory of %d rows exceeds the %d-row bound", len(auctions), maxRows)
	}
	// Consider unanswered auctions in stable oldest-first order.
	sort.SliceStable(auctions, func(i, j int) bool { return auctions[i].Id < auctions[j].Id })
	offers, err := w.bot.readOffers(ctx, pin.Account)
	if err != nil {
		return err
	}
	markets, err := discoverMarkets(ctx, p.Chain, pin, w.policy, auctions, offers)
	if err != nil {
		return err
	}
	// Every managed role participates, including the recipe's funding sources.
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return err
	}
	var leaves []common.Hash
	for _, slot := range inspection.Slots {
		leaves = append(leaves, slot.Leaves...)
	}
	return w.evaluate(ctx, p.Chain, pin, markets, auctions, offers, func(ctx context.Context) error {
		return w.validateOfferAuthorization(ctx, prepared[0], leaves)
	})
}

// Offers precede mint authorization, so full mint simulation cannot admit them.
// Reauthenticate the committed recipe and all roles without inventing mint data.
func (w *offerWorkflow) validateOfferAuthorization(ctx context.Context, original recipes.PreparedRecipe, leaves []common.Hash) error {
	p := w.planner
	fresh, err := recipes.DiscoverBatch(ctx, p.Chain, []common.Address{original.Snapshot.Executor}, []common.Address{original.Snapshot.Account}, p.Caller, recipes.PreparationOptions{ChainID: p.ChainID, Cache: p.RecipeCache})
	if err != nil {
		return err
	}
	a, b := original.Snapshot, fresh[0].Snapshot
	if changed := authorizationChange(a, b); changed != "" {
		return errors.Errorf("3f offer authorization changed (%s); prepare again", changed)
	}
	binding := accountbinding.NewMerkleAccount()
	requests := make([]protocolio.ReadRequest, len(leaves))
	for i, leaf := range leaves {
		requests[i] = protocolio.ContractRead(b.Account, binding.PackHasRole(leaf, b.Executor), new(big.Int).SetUint64(b.BlockNumber), true)
	}
	roles, err := protocolio.ReadGroup(ctx, p.Chain, requests...)
	if err != nil {
		return err
	}
	for i := range leaves {
		result := roles.At(i)
		if result.Err != nil {
			return errors.Errorf("3f read offer leaf role %s: %w", leaves[i].Hex(), result.Err)
		}
		allowed, err := binding.UnpackHasRole(result.Data)
		if err != nil {
			return errors.Errorf("3f decode offer leaf role: %w", err)
		}
		if !allowed {
			return errors.Errorf("3f executor %s lacks offer leaf role %s on account %s", b.Executor.Hex(), leaves[i].Hex(), b.Account.Hex())
		}
	}
	sender := p.Sender
	if sender == (common.Address{}) {
		sender = p.Caller
	}
	// Offers have no mint inputs yet. Check the execution grant without simulating
	// an invented mint; the shared planner validates the actual forwarded mint later.
	if _, err := recipes.RouteExecution(ctx, p.Chain, b, sender, recipeexecutor.NewRecipeExecutor().PackExecute(nil, nil, nil, nil)); err != nil {
		return errors.Errorf("3f offer execution authorization: %w", err)
	}
	return nil
}

// authorizationChange names how the fresh snapshot b differs from the prepared one a, "" when it does not.
func authorizationChange(a, b recipes.Snapshot) string {
	switch {
	case a.Executor != b.Executor:
		return fmt.Sprintf("executor %s became %s", a.Executor.Hex(), b.Executor.Hex())
	case a.Account != b.Account:
		return fmt.Sprintf("account %s became %s", a.Account.Hex(), b.Account.Hex())
	case a.Caller != b.Caller:
		return fmt.Sprintf("caller %s became %s", a.Caller.Hex(), b.Caller.Hex())
	case a.RecipeHash != b.RecipeHash:
		return fmt.Sprintf("recipe hash %s became %s", a.RecipeHash.Hex(), b.RecipeHash.Hex())
	case a.RecipeBlock != b.RecipeBlock:
		return fmt.Sprintf("recipe block %d became %d", a.RecipeBlock, b.RecipeBlock)
	case a.Root != b.Root:
		return fmt.Sprintf("root %s became %s", a.Root.Hex(), b.Root.Hex())
	case a.BlockNumber > b.BlockNumber:
		return fmt.Sprintf("block %d is older than prepared block %d", b.BlockNumber, a.BlockNumber)
	}
	return ""
}

// evaluate is owned by Run. Every membership result, including rejection,
// belongs to this exact pin and is discarded after the pass.
func (w *offerWorkflow) evaluate(ctx context.Context, chain provider.Reader, pin provider.Snapshot, markets []mintMarket, auctions []threef.AuctionDto, offers []threef.OfferDto, validate func(context.Context) error) error {
	if len(offers) > maxRows || len(auctions) > maxRows {
		return errors.Errorf("3F offer inventory of %d auctions and %d offers exceeds the %d-row bound", len(auctions), len(offers), maxRows)
	}
	byID, err := indexAuctions(auctions)
	if err != nil {
		return err
	}
	live, blocked, err := inventory(pin.Account, offers, byID)
	if err != nil {
		return err
	}
	observations := make([]observation, len(markets))
	memberships := make([]map[common.Address]bool, len(markets))
	var pending []*intent
	if w.bot.Send {
		pending = w.bot.pending
	}
	// One debug line per pass names the ineligible requests and the policy's revert reasons.
	ineligible := newSkipLog(ctx)
	defer func() {
		if summary := ineligible.flush("3F requests ineligible"); summary != "" {
			observability.Decline(ctx, "request_ineligible", summary)
		}
	}()
	check := func(i int, request common.Address) error {
		if _, known := memberships[i][request]; known {
			return nil
		}
		err := eligible(ctx, chain, pin, markets[i], request, observations[i].MinimumYield)
		if err != nil && !errors.Is(err, errIneligible) {
			return err
		}
		if err != nil {
			ineligible.addf("market %s/%s: %v", markets[i].Asset.Hex(), markets[i].UnderlyingAsset.Hex(), err)
		}
		memberships[i][request] = err == nil
		return nil
	}
	run := func(i int, create bool) (bool, error) {
		if validate != nil {
			if err := validate(ctx); err != nil {
				return false, err
			}
		}
		observed := observations[i]
		market := offerMarket{ChainID: pin.ChainID.Uint64(), Account: pin.Account, Executor: pin.Executor, Policy: observed.Market.Policy, RecipeHash: pin.RecipeHash, Asset: observed.Market.Asset, UnderlyingAsset: observed.Market.UnderlyingAsset, Budget: observed.Budget, MinimumYield1e8: observed.MinimumYield, MaxPrincipal: observed.MaxPrincipal, AvailableCash: observed.AvailableCash, EligibleRequests: memberships[i]}
		return w.bot.tickInventory(ctx, market, auctions, offers, create)
	}
	// Existing commitments are maintained before any optional new-bid simulation.
	// A mutation invalidates the shared inventory and ends this pass immediately.
	for i, market := range markets {
		observed, err := observePolicy(ctx, chain, pin, market)
		if err != nil {
			return err
		}
		observations[i] = observed
		memberships[i] = make(map[common.Address]bool)
		if observed.Budget.Sign() > 0 {
			for _, previous := range pending {
				if previous.OfferID == 0 && previous.Policy == market.Policy && previous.Asset == market.Asset && previous.UnderlyingAsset == market.UnderlyingAsset {
					if err := check(i, previous.Request); err != nil {
						return err
					}
				}
			}
			for _, offer := range live {
				if offer.asset != market.Asset {
					continue
				}
				if err := check(i, common.HexToAddress(offer.dto.RequestId)); err != nil {
					return err
				}
			}
		}
		if changed, err := run(i, false); err != nil || changed {
			return err
		}
	}
	type candidate struct {
		market  int
		request common.Address
	}
	candidates := make([]candidate, 0)
	queued := make([]map[common.Address]bool, len(markets))
	for i := range queued {
		queued[i] = make(map[common.Address]bool)
	}
	for _, auction := range auctions {
		if !strings.EqualFold(auction.Status, "OPEN") && !strings.EqualFold(auction.Status, "SOLVABLE") {
			continue
		}
		id, _ := dtoID(auction.Id)
		if blocked[id] {
			continue
		}
		asset, err := depositAsset(auction)
		if err != nil {
			return err
		}
		for i, observed := range observations {
			request := common.HexToAddress(auction.RequestId)
			if observed.Budget.Sign() > 0 && observed.AvailableCash.Sign() > 0 && observed.Market.Asset == asset && !queued[i][request] {
				queued[i][request] = true
				candidates = append(candidates, candidate{i, request})
			}
		}
	}
	for _, selected := range candidates {
		if err := check(selected.market, selected.request); err != nil {
			return err
		}
	}
	// Complete eligibility for every unanswered auction before creating an offer.
	// Each market needs one bidding pass over the complete membership inventory.
	attempted := make([]bool, len(markets))
	for _, selected := range candidates {
		if attempted[selected.market] || !memberships[selected.market][selected.request] {
			continue
		}
		attempted[selected.market] = true
		if changed, err := run(selected.market, true); err != nil || changed {
			return err
		}
	}
	for i, observed := range observations {
		if attempted[i] {
			continue
		}
		var reason string
		switch {
		case observed.Budget.Sign() <= 0:
			reason = "policy budget is 0"
		case observed.AvailableCash.Sign() <= 0:
			reason = "available cash is 0"
		default:
			reason = "no open, unblocked auction has an eligible request for this asset"
		}
		observability.Log(ctx).V(1).Info("3F market not bidding", "asset", observed.Market.Asset.Hex(),
			"underlyingAsset", observed.Market.UnderlyingAsset.Hex(), "reason", reason)
		observability.Decline(ctx, "market_skipped", fmt.Sprintf("market %s/%s: %s",
			observed.Market.Asset.Hex(), observed.Market.UnderlyingAsset.Hex(), reason))
	}
	return nil
}
