package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

const connectorDiscountPrecision int64 = 100_000_000
const connectorQuoteReads = 7

type connectorQuoteObservation struct {
	account                  common.Address
	capacity, minimum, price *big.Int
}

// readConnectorCandidates observes each connector once per quote, independent of its discount
// alternatives. Reactor delivers tokenIn during the atomic fill; capacity is the recipe getter.
// Header/token metadata and batch transport errors invalidate the shared observation. A failed
// connector member only excludes that connector, preserving healthy connector and native routes.
func (r *reader) readConnectorCandidates(
	ctx context.Context, inventory []solverInventory, tokenIn, tokenOut common.Address,
	amountIn *big.Int, pending pendingReservations,
) ([]liquidlane.QuoteCandidate, error) {
	if len(inventory) == 0 {
		return nil, nil
	}
	header, err := r.chain.HeaderByNumber(ctx, nil)
	if err != nil {
		return nil, errors.Errorf("connector quote block: %w", err)
	}
	if header == nil || header.Number == nil || !header.Number.IsUint64() {
		return nil, errors.New("invalid connector quote block")
	}
	inDecimals, err := r.ll.TokenDecimals(ctx, tokenIn)
	if err != nil {
		return nil, errors.Errorf("connector tokenIn decimals: %w", err)
	}
	outDecimals, err := r.ll.TokenDecimals(ctx, tokenOut)
	if err != nil {
		return nil, errors.Errorf("connector output decimals: %w", err)
	}
	probe, ok := liquidlane.OraclePriceProbe(inDecimals, outDecimals)
	if !ok {
		return nil, errors.New("connector oracle decimals unsupported")
	}
	binding := connector.NewLiquidLaneConnector()
	indices := make(map[common.Address]int, len(inventory))
	requests := make([]protocolio.ReadRequest, 0, len(inventory)*connectorQuoteReads)
	for _, item := range inventory {
		if _, found := indices[item.Adapter]; found || !validConnectorQuoteItem(item, outDecimals) {
			continue
		}
		indices[item.Adapter] = len(requests)
		for _, data := range [][]byte{binding.PackEip712Domain(), binding.PackOutputAsset(), binding.PackAccount(), binding.PackGetMaxAssets(tokenIn), binding.PackMinDiscount(tokenIn), binding.PackGetAmountOut(tokenIn, probe), binding.PackGetAmountOut(tokenIn, amountIn)} {
			// Keep each eth_call isolated with its original zero caller. getMaxAssets runs a
			// rollback-only funding preview: aggregating it would change caller/transient state.
			requests = append(requests, protocolio.ContractRead(item.Adapter, data, header.Number, false))
		}
	}
	group, err := protocolio.ReadGroup(ctx, r.chain, requests...)
	if err != nil {
		return nil, errors.Errorf("connector quote observations: %w", err)
	}
	observed := make(map[common.Address]connectorQuoteObservation, len(indices))
	for address, index := range indices {
		observation, err := r.decodeConnectorObservation(group, index, address, tokenOut, amountIn, inDecimals, outDecimals)
		if err != nil {
			observability.Log(ctx).V(1).Info("connector quote route unavailable", "connector", address.Hex(), "reason", err.Error())
			continue
		}
		observed[address] = observation
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	current := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		observation, found := observed[item.Adapter]
		if !found || !validConnectorQuoteItem(item, outDecimals) || item.Discount.Cmp(observation.minimum) < 0 {
			continue
		}
		item.TokenInDecimals = inDecimals
		item.Vault = observation.account
		item.CapacityID = liquidlane.NewCapacityID(r.chainID, observation.account, tokenOut)
		item.Price = observation.price
		if observation.capacity.Cmp(item.MaxAssets) < 0 {
			item.MaxAssets = observation.capacity
		}
		item.BlockNumber = header.Number.Uint64()
		current = append(current, item)
	}
	var reservations liquidlane.CapacityReservations
	if pending != nil {
		reservations = pending(current)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(current, reservations, 0)
	out := make([]liquidlane.QuoteCandidate, 0, len(allocated))
	for _, item := range allocated {
		if candidate := liquidgreedy.NewQuoteCandidate(item, item.MaxAssets); candidate != nil {
			out = append(out, *candidate)
		}
	}
	return out, nil
}

func validConnectorQuoteItem(item solverInventory, outDecimals int) bool {
	return item.DiscountID != nil && item.Discount != nil && item.DiscountPrecision == connectorDiscountPrecision &&
		item.Discount.Sign() >= 0 && item.Discount.Cmp(big.NewInt(connectorDiscountPrecision)) < 0 &&
		item.MaxAssets != nil && item.MaxAssets.Sign() >= 0 && item.TokenOutDecimals == outDecimals
}

func (r *reader) decodeConnectorObservation(group *protocolio.ReadResults, index int, address, tokenOut common.Address, amountIn *big.Int, inDecimals, outDecimals int) (connectorQuoteObservation, error) {
	var observation connectorQuoteObservation
	data := make([][]byte, connectorQuoteReads)
	for offset := range data {
		result := group.At(index + offset)
		if result.Err != nil {
			return observation, errors.Errorf("connector getter %d: %w", offset, result.Err)
		}
		data[offset] = result.Data
	}
	binding := connector.NewLiquidLaneConnector()
	domain, err := binding.UnpackEip712Domain(data[0])
	if err != nil || domain.Fields != [1]byte{0x0f} || domain.Name != "LiquidLaneConnector" || domain.Version != "1" || domain.ChainId == nil || domain.ChainId.Cmp(big.NewInt(r.chainID)) != 0 || domain.VerifyingContract != address || domain.Salt != [32]byte{} || len(domain.Extensions) != 0 {
		return observation, errors.New("invalid connector domain")
	}
	output, err := binding.UnpackOutputAsset(data[1])
	if err != nil || output != tokenOut {
		return observation, errors.New("connector outputAsset mismatch")
	}
	observation.account, err = binding.UnpackAccount(data[2])
	if err != nil || observation.account == (common.Address{}) {
		return observation, errors.New("invalid connector account")
	}
	observation.capacity, err = binding.UnpackGetMaxAssets(data[3])
	if err != nil || observation.capacity == nil || observation.capacity.Sign() < 0 {
		return observation, errors.New("invalid connector getMaxAssets")
	}
	observation.minimum, err = binding.UnpackMinDiscount(data[4])
	if err != nil || observation.minimum == nil || observation.minimum.Sign() < 0 || observation.minimum.Cmp(big.NewInt(connectorDiscountPrecision)) > 0 {
		return observation, errors.New("invalid connector minDiscount")
	}
	observation.price, err = binding.UnpackGetAmountOut(data[5])
	if err != nil || observation.price == nil || observation.price.Sign() <= 0 {
		return observation, errors.New("invalid connector price")
	}
	gross, err := binding.UnpackGetAmountOut(data[6])
	if err != nil || gross == nil || gross.Cmp(liquidlane.AmountOutForRate(amountIn, observation.price, inDecimals, outDecimals)) != 0 {
		return observation, errors.New("connector oracle quote mismatch")
	}
	return observation, nil
}
