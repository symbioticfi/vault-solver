package threef

import (
	"context"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
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
	planner  *keeper.Planner
	strategy keeper.Strategy
	api      *apiClient
	bot      *offerBot
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
	observed, err := observe(ctx, p.Chain, pin, recipe, tree)
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
	membership := map[common.Address]bool{}
	if observed.Budget.Sign() > 0 {
		for _, auction := range auctions {
			if !strings.EqualFold(auction.Status, "OPEN") && !strings.EqualFold(auction.Status, "SOLVABLE") || !common.IsHexAddress(auction.RequestId) {
				continue
			}
			request := common.HexToAddress(auction.RequestId)
			if membership[request] {
				continue
			}
			if err := eligible(ctx, p.Chain, pin, observed.Market, request, observed.MinimumYield); err != nil {
				if errors.Is(err, errIneligible) {
					continue
				}
				return err
			}
			membership[request] = true
		}
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
	if err := recipes.VerifyRoles(ctx, p.Chain, snapshot, leaves); err != nil {
		return err
	}
	if err := recipes.ValidateSnapshot(ctx, p.Chain, snapshot, p.Caller); err != nil {
		return err
	}
	return w.bot.Tick(ctx, offerMarket{ChainID: p.ChainID.Uint64(), Account: snapshot.Account, Executor: snapshot.Executor, RecipeHash: snapshot.RecipeHash, Asset: observed.Market.Asset, UnderlyingAsset: observed.Market.UnderlyingAsset, Budget: observed.Budget, MinimumYield1e8: observed.MinimumYield, MaxPrincipal: observed.MaxPrincipal, AvailableCash: observed.AvailableCash, EligibleRequests: membership}, auctions)
}
