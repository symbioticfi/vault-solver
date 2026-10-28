package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidgreedy "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies/greedy"
)

const connectorDiscountPrecision int64 = 100_000_000

// readConnectorCandidates reads the explicit backend connector catalog through its canonical
// contract surface. Reactor delivers tokenIn during the atomic fill, so an empty connector
// balance before the quote does not constrain admission. The backend derives recipe funding
// capacity; this reader clips it by the current connector policy budget and shared reservations.
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
	block := header.Number
	binding := connector.NewLiquidLaneConnector()
	call := func(address common.Address, data []byte) ([]byte, error) {
		return r.chain.CallContract(ctx, ethereum.CallMsg{To: &address, Data: data}, block)
	}
	inDecimals, err := r.ll.TokenDecimals(ctx, tokenIn)
	if err != nil {
		return nil, errors.Errorf("connector tokenIn decimals: %w", err)
	}
	current := make([]liquidlane.Inventory, 0, len(inventory))
	for _, item := range inventory {
		if item.DiscountID == nil || item.Discount == nil || item.DiscountPrecision != connectorDiscountPrecision {
			continue
		}
		domainData, err := call(item.Adapter, binding.PackEip712Domain())
		if err != nil {
			return nil, errors.Errorf("connector eip712Domain: %w", err)
		}
		domain, err := binding.UnpackEip712Domain(domainData)
		if err != nil || domain.Fields != [1]byte{0x0f} || domain.Name != "LiquidLaneConnector" || domain.Version != "1" || domain.ChainId == nil || domain.ChainId.Cmp(big.NewInt(r.chainID)) != 0 || domain.VerifyingContract != item.Adapter || domain.Salt != [32]byte{} || len(domain.Extensions) != 0 {
			return nil, errors.New("invalid connector domain")
		}
		outputData, err := call(item.Adapter, binding.PackOutputAsset())
		if err != nil {
			return nil, errors.Errorf("connector outputAsset: %w", err)
		}
		output, err := binding.UnpackOutputAsset(outputData)
		if err != nil || output != tokenOut {
			return nil, errors.New("connector outputAsset mismatch")
		}
		outDecimals, err := r.ll.TokenDecimals(ctx, output)
		if err != nil {
			return nil, errors.Errorf("connector output decimals: %w", err)
		}
		if outDecimals != item.TokenOutDecimals {
			return nil, errors.New("connector output decimals mismatch")
		}
		accountData, err := call(item.Adapter, binding.PackAccount())
		if err != nil {
			return nil, errors.Errorf("connector account: %w", err)
		}
		account, err := binding.UnpackAccount(accountData)
		if err != nil || account == (common.Address{}) {
			return nil, errors.New("invalid connector account")
		}
		capacityData, err := call(item.Adapter, binding.PackRemainingAssets(tokenIn))
		if err != nil {
			return nil, errors.Errorf("connector remainingAssets: %w", err)
		}
		capacity, err := binding.UnpackRemainingAssets(capacityData)
		if err != nil {
			return nil, errors.Errorf("connector remainingAssets decode: %w", err)
		}
		minData, err := call(item.Adapter, binding.PackMinDiscount(tokenIn))
		if err != nil {
			return nil, errors.Errorf("connector minDiscount: %w", err)
		}
		minimum, err := binding.UnpackMinDiscount(minData)
		if err != nil {
			return nil, errors.Errorf("connector minDiscount decode: %w", err)
		}
		if item.Discount.Cmp(minimum) < 0 || item.Discount.Sign() < 0 || item.Discount.Cmp(big.NewInt(connectorDiscountPrecision)) >= 0 {
			continue
		}
		probe, ok := liquidlane.OraclePriceProbe(inDecimals, outDecimals)
		if !ok {
			return nil, errors.New("connector oracle decimals unsupported")
		}
		priceData, err := call(item.Adapter, binding.PackGetAmountOut(tokenIn, probe))
		if err != nil {
			return nil, errors.Errorf("connector getAmountOut: %w", err)
		}
		price, err := binding.UnpackGetAmountOut(priceData)
		if err != nil {
			return nil, errors.Errorf("connector price decode: %w", err)
		}
		// Also require the actual requested-amount oracle read to succeed (including freshness).
		amountData, err := call(item.Adapter, binding.PackGetAmountOut(tokenIn, amountIn))
		if err != nil {
			return nil, errors.Errorf("connector getAmountOut amount: %w", err)
		}
		gross, err := binding.UnpackGetAmountOut(amountData)
		if err != nil || gross.Cmp(liquidlane.AmountOutForRate(amountIn, price, inDecimals, outDecimals)) != 0 {
			return nil, errors.New("connector oracle quote mismatch")
		}
		item.TokenInDecimals = inDecimals
		item.Vault = account
		item.CapacityID = liquidlane.NewCapacityID(r.chainID, account, output)
		item.Price = price
		if capacity.Cmp(item.MaxAssets) < 0 {
			item.MaxAssets = capacity
		}
		// Backend funding remains as old as its observation, even when policy was read now.
		if item.BlockNumber > block.Uint64() {
			item.BlockNumber = block.Uint64()
		}
		current = append(current, item)
	}
	canonical, err := r.chain.HeaderByNumber(ctx, block)
	if err != nil || canonical == nil || canonical.Number == nil || canonical.Number.Cmp(block) != 0 || canonical.Hash() != header.Hash() {
		return nil, errors.New("connector quote block reorganized")
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
