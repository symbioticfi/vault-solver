package connector

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

// Precision is the connector's signed discount denominator; native adapters use 1e6.
// PrecisionText is its wire form in discount listings and resolved terms.
const (
	Precision     int64 = 100_000_000
	PrecisionText       = "100000000"
)

// Caller reads contract state at a block.
type Caller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// Offer is one advertised connector discount on a configured recipe route. MaxAssets is the
// upstream funding capacity of Account in TokenOut, observed at BlockNumber.
type Offer struct {
	Venue          common.Address // LiquidLaneConnector
	RecipeExecutor common.Address
	Account        common.Address
	TokenIn        common.Address
	TokenOut       common.Address

	TokenInDecimals  int
	TokenOutDecimals int

	DiscountID  common.Hash
	Discount    *big.Int
	MaxAssets   *big.Int
	BlockNumber uint64
	ValidUntil  time.Time
}

// ReadInventory prices offers at block, or at the latest head when block is nil. The
// settlement contract funds the connector inside its callback, so an empty connector balance
// does not constrain a quote; capacity is the advertised funding clipped by the connector's
// remaining budget. An offer its connector contradicts or cannot price is dropped and reported
// in skipped, next to the inventory that priced; err reports a read that leaves no inventory to
// trust at all (the block, cancellation). The block is not re-checked against the canonical
// chain: inventory only feeds quotes that are re-simulated at a fresh head and enforced on-chain.
func ReadInventory(ctx context.Context, chain recipes.Reader, chainID int64, offers []Offer, block *big.Int) (inventory []liquidlane.Inventory, skipped, err error) {
	if len(offers) == 0 {
		return nil, nil, nil
	}
	if block == nil {
		header, err := chain.HeaderByNumber(ctx, nil)
		if err != nil {
			return nil, nil, errors.Errorf("connector inventory block: %w", err)
		}
		if header != nil {
			block = header.Number
		}
	}
	if block == nil || !block.IsUint64() {
		return nil, nil, errors.New("invalid connector inventory block")
	}
	var dropped []error
	inventory = make([]liquidlane.Inventory, 0, len(offers))
	for _, offer := range offers {
		item, err := offer.Read(ctx, chain, chainID, block)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			dropped = append(dropped, errors.Errorf("connector %s discount %s: %w", offer.Venue.Hex(), offer.DiscountID.Hex(), err))
			continue
		}
		if item != nil {
			inventory = append(inventory, *item)
		}
	}
	return inventory, errors.Join(dropped...), nil
}

// Read prices the offer against its connector's policy at block: the connector must be the
// canonical contract on this chain, pay TokenOut from Account and price TokenIn. Capacity is
// MaxAssets clipped by the connector's remaining budget. Read returns nil when the connector's
// current minimum discount outbids the offer.
func (o Offer) Read(ctx context.Context, chain Caller, chainID int64, block *big.Int) (*liquidlane.Inventory, error) {
	if o.Discount == nil || o.Discount.Sign() < 0 || o.Discount.Cmp(big.NewInt(Precision)) >= 0 || o.MaxAssets == nil || o.MaxAssets.Sign() < 0 {
		return nil, errors.New("invalid advertised discount or capacity")
	}
	call := func(data []byte) ([]byte, error) {
		return chain.CallContract(ctx, ethereum.CallMsg{To: &o.Venue, Data: data}, block)
	}
	raw, err := call(connectorBinding.PackEip712Domain())
	if err != nil {
		return nil, errors.Errorf("eip712Domain: %w", err)
	}
	domain, err := connectorBinding.UnpackEip712Domain(raw)
	if err != nil || domain.Fields != [1]byte{0x0f} || domain.Name != "LiquidLaneConnector" || domain.Version != "1" ||
		domain.ChainId == nil || domain.ChainId.Cmp(big.NewInt(chainID)) != 0 || domain.VerifyingContract != o.Venue ||
		domain.Salt != [32]byte{} || len(domain.Extensions) != 0 {
		return nil, errors.New("invalid connector domain")
	}
	if raw, err = call(connectorBinding.PackOutputAsset()); err != nil {
		return nil, errors.Errorf("outputAsset: %w", err)
	}
	if output, err := connectorBinding.UnpackOutputAsset(raw); err != nil || output != o.TokenOut {
		return nil, errors.New("outputAsset mismatch")
	}
	if raw, err = call(connectorBinding.PackAccount()); err != nil {
		return nil, errors.Errorf("account: %w", err)
	}
	if account, err := connectorBinding.UnpackAccount(raw); err != nil || account == (common.Address{}) || account != o.Account {
		return nil, errors.New("account mismatch")
	}
	if raw, err = call(connectorBinding.PackRemainingAssets(o.TokenIn)); err != nil {
		return nil, errors.Errorf("remainingAssets: %w", err)
	}
	capacity, err := connectorBinding.UnpackRemainingAssets(raw)
	if err != nil {
		return nil, errors.Errorf("remainingAssets decode: %w", err)
	}
	if raw, err = call(connectorBinding.PackMinDiscount(o.TokenIn)); err != nil {
		return nil, errors.Errorf("minDiscount: %w", err)
	}
	minimum, err := connectorBinding.UnpackMinDiscount(raw)
	if err != nil {
		return nil, errors.Errorf("minDiscount decode: %w", err)
	}
	if o.Discount.Cmp(minimum) < 0 {
		return nil, nil
	}
	probe, ok := liquidlane.OraclePriceProbe(o.TokenInDecimals, o.TokenOutDecimals)
	if !ok {
		return nil, errors.New("oracle decimals unsupported")
	}
	if raw, err = call(connectorBinding.PackGetAmountOut(o.TokenIn, probe)); err != nil {
		return nil, errors.Errorf("getAmountOut: %w", err)
	}
	price, err := connectorBinding.UnpackGetAmountOut(raw)
	if err != nil || price.Sign() <= 0 {
		return nil, errors.New("invalid connector price")
	}
	route := liquidlane.NewRoute(chainID, o.Venue, o.Account, o.TokenIn, o.TokenOut, o.TokenInDecimals, o.TokenOutDecimals)
	item := liquidlane.DiscountInventory(route, o.MaxAssets, nil, o.DiscountID, o.ValidUntil)
	if capacity.Cmp(item.MaxAssets) < 0 {
		item.MaxAssets = capacity
	}
	item.RecipeExecutor, item.DiscountPrecision = o.RecipeExecutor, Precision
	item.Discount, item.AdapterMinDiscount = new(big.Int).Set(o.Discount), minimum
	item.Price = price
	// Upstream funding stays as old as its observation, even though policy was read now.
	item.BlockNumber = min(o.BlockNumber, block.Uint64())
	return &item, nil
}
