package threef

import (
	"context"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
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

func (p *mintPlanning) Validate(ctx context.Context, plan keeper.Plan) error {
	if err := p.Planner.Validate(ctx, plan); err != nil {
		return err
	}
	if len(plan.Outputs) != 1 {
		return errors.New("3f plan requires one provider")
	}
	if err := p.runtime.ValidateLease(ctx, plan.Snapshot, plan.Outputs[0]); err != nil {
		return err
	}
	return nil
}

type offerWorkflow struct {
	planner   *keeper.Planner
	strategy  keeper.Strategy
	api       *apiClient
	bot       *offerBot
	maxChecks int
	cursor    int
}

func (w *offerWorkflow) Tick(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "threef-merkle.offers.tick", observability.AttrAdapter.String(w.strategy.Config.Account))
	defer func() { end(err) }()
	p := w.planner
	snapshot, recipe, err := recipes.DiscoverForAccount(ctx, p.Chain, common.HexToAddress(w.strategy.Config.Executor), common.HexToAddress(w.strategy.Config.Account), nil)
	if err != nil {
		return err
	}
	raw, err := p.PermissionSource.PermissionTree(ctx, p.ChainID, snapshot)
	if err != nil {
		return err
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		return err
	}
	if tree.ChainID == nil || tree.ChainID.Cmp(p.ChainID) != 0 || tree.Root != snapshot.Root || tree.Account != nil && *tree.Account != snapshot.Account {
		return errors.New("3f permission scope mismatch")
	}
	pin := provider.Snapshot{ChainID: new(big.Int).Set(p.ChainID), BlockNumber: new(big.Int).SetUint64(snapshot.BlockNumber), BlockHash: snapshot.BlockHash, Timestamp: snapshot.Timestamp, Executor: snapshot.Executor, Account: snapshot.Account, Caller: snapshot.Caller, RecipeHash: snapshot.RecipeHash, Root: snapshot.Root}
	markets, err := inspect(pin, recipe, tree)
	if err != nil {
		return err
	}
	auctions, err := w.api.Auctions(ctx)
	if err != nil {
		return err
	}
	if len(auctions) > 4096 {
		return errors.New("3f auction inventory exceeds 4096-row bound")
	}
	// Stable oldest-first order is resumed across bounded optional scans.
	sort.SliceStable(auctions, func(i, j int) bool { return auctions[i].Id < auctions[j].Id })
	// Every managed role participates, including the recipe's funding sources.
	inspection, err := recipes.Inspect(recipe)
	if err != nil {
		return err
	}
	var leaves []common.Hash
	for _, slot := range inspection.Slots {
		leaves = append(leaves, slot.Leaves...)
	}
	if err := recipes.VerifyRoles(ctx, p.Chain, snapshot, leaves); err != nil {
		return err
	}
	if err := recipes.ValidateSnapshot(ctx, p.Chain, snapshot, p.Caller); err != nil {
		return err
	}
	return w.evaluate(ctx, p.Chain, pin, recipe, markets, auctions, func(ctx context.Context) error {
		return recipes.ValidateSnapshot(ctx, p.Chain, snapshot, p.Caller)
	})
}

// evaluate is owned by Run. Only the bounded optional scan cursor survives a
// tick; every membership result, including rejection, belongs to this exact pin.
func (w *offerWorkflow) evaluate(ctx context.Context, chain provider.Reader, pin provider.Snapshot, recipe recipes.Recipe, markets []mintMarket, auctions []threef.AuctionDto, validate func(context.Context) error) error {
	offers, err := w.bot.readOffers(ctx, pin.Account)
	if err != nil {
		return err
	}
	if len(offers) > 4096 || len(auctions) > 4096 {
		return errors.New("3F offer inventory exceeds 4096-row bound")
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
	check := func(i int, request common.Address) (bool, error) {
		if result, known := memberships[i][request]; known {
			return result, nil
		}
		err := eligible(ctx, chain, pin, markets[i], request, observations[i].MinimumYield)
		if err != nil && !errors.Is(err, errIneligible) {
			return false, err
		}
		memberships[i][request] = err == nil
		return err == nil, nil
	}
	run := func(i int, create bool) (bool, error) {
		if validate != nil {
			if err := validate(ctx); err != nil {
				return false, err
			}
		}
		observed := observations[i]
		market := offerMarket{ChainID: pin.ChainID.Uint64(), Account: pin.Account, Executor: pin.Executor, RecipeHash: pin.RecipeHash, Asset: observed.Market.Asset, UnderlyingAsset: observed.Market.UnderlyingAsset, Budget: observed.Budget, MinimumYield1e8: observed.MinimumYield, MaxPrincipal: observed.MaxPrincipal, AvailableCash: observed.AvailableCash, EligibleRequests: memberships[i]}
		return w.bot.tickInventory(ctx, market, auctions, offers, create)
	}
	// Existing commitments are maintained before any optional new-bid simulation.
	// A mutation invalidates the shared inventory and ends this pass immediately.
	for i, market := range markets {
		observed, err := observe(ctx, chain, pin, recipe, market)
		if err != nil {
			return err
		}
		observations[i] = observed
		memberships[i] = make(map[common.Address]bool)
		if observed.Budget.Sign() > 0 {
			for _, offer := range live {
				if offer.asset != market.Asset {
					continue
				}
				if _, err := check(i, common.HexToAddress(offer.dto.RequestId)); err != nil {
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
	if len(candidates) == 0 {
		w.cursor = 0
		return nil
	}
	start, checks := w.cursor%len(candidates), 0
	for visited := 0; visited < len(candidates); visited++ {
		index := (start + visited) % len(candidates)
		selected := candidates[index]
		if _, known := memberships[selected.market][selected.request]; !known {
			if checks >= w.maxChecks {
				return nil
			}
			checks++
		}
		accepted, err := check(selected.market, selected.request)
		if err != nil {
			return err
		}
		w.cursor = (index + 1) % len(candidates)
		if accepted {
			if changed, err := run(selected.market, true); err != nil || changed {
				w.cursor = 0
				return err
			}
		}
	}
	w.cursor = 0
	return nil
}
