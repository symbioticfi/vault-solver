package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidconnector "github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

const connectorDiscountPrecision = liquidconnector.Precision

// readConnectorCandidates reads the explicit backend connector catalog through its canonical
// contract surface. Reactor delivers tokenIn during the atomic fill, so an empty connector
// balance before the quote does not constrain admission. The backend derives recipe funding
// capacity; this reader clips it by the current connector policy budget and shared reservations.
func (r *reader) readConnectorCandidates(
	ctx context.Context, inventory []solverInventory, tokenIn, tokenOut common.Address,
	amountIn *big.Int, pending pendingReservations, trace liquidstrategies.DecisionTrace,
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
	block := header.Number
	binding := connector.NewLiquidLaneConnector()
	inDecimals, err := r.ll.TokenDecimals(ctx, tokenIn)
	if err != nil {
		return nil, errors.Errorf("connector tokenIn decimals: %w", err)
	}
	current := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if item.DiscountID == nil || item.Discount == nil || item.DiscountPrecision != connectorDiscountPrecision ||
			item.Discount.Sign() < 0 || item.Discount.Cmp(big.NewInt(connectorDiscountPrecision)) >= 0 {
			trace.Decline("inventory", "connector discount is not a signed discount in [0, precision)", "route", item.ID,
				"discountId", item.DiscountID, "discount", item.Discount,
				"precision", item.DiscountPrecision, "wantPrecision", connectorDiscountPrecision)
			continue
		}
		outDecimals, err := r.ll.TokenDecimals(ctx, tokenOut)
		if err != nil {
			return nil, errors.Errorf("connector output decimals: %w", err)
		}
		if outDecimals != item.TokenOutDecimals {
			return nil, errors.New("connector output decimals mismatch")
		}
		// One quote fails closed on any connector that cannot be read or contradicts the backend.
		read, err := liquidconnector.Offer{
			Venue: item.Adapter, RecipeExecutor: item.RecipeExecutor, Account: item.Vault, TokenIn: tokenIn, TokenOut: tokenOut,
			TokenInDecimals: inDecimals, TokenOutDecimals: outDecimals, DiscountID: *item.DiscountID, Discount: item.Discount,
			MaxAssets: item.MaxAssets, BlockNumber: item.BlockNumber, ValidUntil: item.ValidUntil,
		}.Read(ctx, r.chain, r.chainID, block)
		if err != nil {
			return nil, errors.Errorf("connector %w", err)
		}
		if read == nil {
			trace.Decline("inventory", "connector minimum discount exceeds signed discount", "route", item.ID,
				"discountId", item.DiscountID, "discount", item.Discount)
			continue
		}
		// Also require the actual requested-amount oracle read to succeed (including freshness).
		amountData, err := r.chain.CallContract(ctx, ethereum.CallMsg{To: &item.Adapter, Data: binding.PackGetAmountOut(tokenIn, amountIn)}, block)
		if err != nil {
			return nil, errors.Errorf("connector getAmountOut amount: %w", err)
		}
		gross, err := binding.UnpackGetAmountOut(amountData)
		if err != nil || gross.Cmp(liquidlane.AmountOutForRate(amountIn, read.Price, inDecimals, outDecimals)) != 0 {
			return nil, errors.New("connector oracle quote mismatch")
		}
		item.TokenInDecimals = inDecimals
		item.CapacityID = read.CapacityID
		item.Price = read.Price
		item.MaxAssets = read.MaxAssets
		// Backend funding remains as old as its observation, even when policy was read now.
		item.BlockNumber = read.BlockNumber
		current = append(current, item)
	}
	var reservations liquidlane.CapacityReservations
	if pending != nil {
		reservations = pending(current)
	}
	allocated := liquidgreedy.AllocateInventoryCapacity(current, reservations, 0)
	declineUnallocated(trace, current, allocated, reservations)
	out := make([]liquidlane.QuoteCandidate, 0, len(allocated))
	for _, item := range allocated {
		candidate := liquidgreedy.NewQuoteCandidate(item, item.MaxAssets)
		if candidate == nil {
			trace.Decline("inventory", "candidate prices no output", "route", item.ID,
				"maxAssets", item.MaxAssets, "price", item.Price, "discount", item.Discount)
			continue
		}
		out = append(out, *candidate)
	}
	return out, nil
}
