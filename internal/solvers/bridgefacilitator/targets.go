package bridgefacilitator

import (
	"context"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

func (solver *Solver) refreshTargetsAndHydrate(ctx context.Context) (err error) {
	timer := observability.StartOperation(solver.metrics.operation(targetRefreshOperation))
	defer func() { timer.Finish(ctx, observability.OutcomeForError(err)) }()
	added, err := solver.refreshTargets(ctx)
	if err != nil {
		return err
	}
	solver.reconcileOffers(ctx, added)
	solver.metrics.state("targets", len(solver.targets))
	return nil
}

func (solver *Solver) refreshTargets(ctx context.Context) ([]Target, error) {
	addresses, err := solver.configuredOrDiscoveredAdapters(ctx)
	if err != nil {
		return nil, err
	}
	addresses = deduplicateAdapters(addresses)
	if len(addresses) == 0 {
		solver.offers.retainAdapters(nil)
		solver.targets = nil
		solver.publishTargets(nil)
		return nil, nil
	}

	resolved, err := solver.reader.resolveAdapters(ctx, addresses, solver.probe)
	if err != nil {
		return nil, err
	}
	previous := make(map[common.Address]struct{}, len(solver.targets))
	for _, target := range solver.targets {
		previous[target.Adapter] = struct{}{}
	}

	kept := make([]Target, 0, len(addresses))
	added := make([]Target, 0, len(addresses))
	for index, address := range addresses {
		candidate := resolved[index]
		if candidate.err != nil {
			if errors.Is(candidate.err, errAdapterUnconfigured) {
				solver.log.V(1).Info("skipping adapter: not configured on-chain",
					"adapter", address.Hex(), "reason", candidate.err.Error())
			} else {
				solver.log.Error(candidate.err, "skipping adapter: resolution failed", "adapter", address.Hex())
			}
			continue
		}
		if !candidate.authorized {
			solver.log.Info("skipping adapter: solver is not an authorized offer signer",
				"adapter", address.Hex(), "solver", solver.signerAddr.Hex(), "offerSigner", candidate.signer.Hex())
			continue
		}
		if !candidate.usable() {
			solver.log.Info("skipping adapter: incomplete resolution", "adapter", address.Hex())
			continue
		}
		target := Target{Adapter: address, Vault: candidate.vault, Collateral: candidate.collateral}
		kept = append(kept, target)
		if _, exists := previous[address]; !exists {
			added = append(added, target)
		}
		solver.log.Info("resolved target",
			"adapter", address.Hex(), "vault", candidate.vault.Hex(), "collateral", candidate.collateral.Hex())
	}

	active := make(map[common.Address]struct{}, len(kept))
	for _, target := range kept {
		active[target.Adapter] = struct{}{}
	}
	solver.offers.retainAdapters(active)
	solver.targets = kept
	solver.publishTargets(kept)
	return added, nil
}

func (solver *Solver) publishTargets(targets []Target) {
	snapshot := append([]Target(nil), targets...)
	solver.redeemable.Store(&snapshot)
}

func (solver *Solver) configuredOrDiscoveredAdapters(ctx context.Context) ([]common.Address, error) {
	if solver.cfg.Targets == nil {
		return solver.reader.factoryAdapters(ctx, solver.cfg.AdapterFactory)
	}
	addresses := make([]common.Address, len(solver.cfg.Targets))
	for index, target := range solver.cfg.Targets {
		addresses[index] = target.Adapter
	}
	return addresses, nil
}
