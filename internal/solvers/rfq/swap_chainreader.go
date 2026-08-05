package rfq

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type swapNonceCheck struct {
	Adapter common.Address
	TokenIn common.Address
	Nonce   *big.Int
}

type swapStateReader interface {
	validateRouter(ctx context.Context, router common.Address) error
	validateAdapters(ctx context.Context, adapters []common.Address, signer common.Address) (map[common.Address]swapDomain, error)
	readFillQuote(ctx context.Context, route liquidlane.Route, amountIn *big.Int) (liquidlane.FillQuote, error)
	readUsedNonces(ctx context.Context, checks []swapNonceCheck) ([]bool, error)
}

type swapChainBackend interface {
	CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error)
	Multicall(ctx context.Context, calls []chain.Call) ([]chain.CallResult, error)
}

type swapLiquidLaneReader interface {
	ReadAuth(ctx context.Context, adapters []common.Address, filler common.Address) ([]liquidlane.Auth, error)
	ReadFillQuotes(ctx context.Context, routes []liquidlane.Route, tokenIn common.Address, amountIn *big.Int) ([]liquidlane.FillQuote, error)
}

type swapOnchainReader struct {
	chain   swapChainBackend
	ll      swapLiquidLaneReader
	chainID int64
}

func newSwapOnchainReader(chainBackend swapChainBackend, ll swapLiquidLaneReader, chainID int64) *swapOnchainReader {
	return &swapOnchainReader{chain: chainBackend, ll: ll, chainID: chainID}
}

func (r *swapOnchainReader) validateRouter(ctx context.Context, router common.Address) error {
	if router == (common.Address{}) {
		return errors.New("swap Router is zero")
	}
	code, err := r.chain.CodeAt(ctx, router, nil)
	if err != nil {
		return errors.Errorf("read swap Router code: %w", err)
	}
	if len(code) == 0 {
		return errors.Errorf("swap Router %s has no deployed bytecode", router.Hex())
	}
	return nil
}

func (r *swapOnchainReader) validateAdapters(
	ctx context.Context,
	addresses []common.Address,
	signer common.Address,
) (map[common.Address]swapDomain, error) {
	addresses = dedupeSwapAddresses(addresses)
	if len(addresses) == 0 {
		return map[common.Address]swapDomain{}, nil
	}
	if signer == (common.Address{}) {
		return nil, errors.New("swap signer is zero")
	}
	auth, err := r.ll.ReadAuth(ctx, addresses, signer)
	if err != nil {
		return nil, errors.Errorf("read swap adapter authorization: %w", err)
	}
	authorized := make(map[common.Address]bool, len(auth))
	for _, item := range auth {
		authorized[item.Adapter] = item.Authorized
	}
	for _, address := range addresses {
		if !authorized[address] {
			return nil, errors.Errorf("swap signer %s is not authorized for adapter %s", signer.Hex(), address.Hex())
		}
	}

	calls := make([]chain.Call, len(addresses))
	for i, address := range addresses {
		data, packErr := swapAdapterBinding.TryPackEip712Domain()
		if packErr != nil {
			return nil, errors.Errorf("pack adapter domain read: %w", packErr)
		}
		calls[i] = chain.Call{Target: address, AllowFailure: true, Data: data}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("read swap adapter domains: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("read swap adapter domains: got %d results, want %d", len(results), len(calls))
	}
	domains := make(map[common.Address]swapDomain, len(addresses))
	for i, result := range results {
		if !result.Success {
			return nil, errors.Errorf("read swap adapter %s domain: call failed", addresses[i].Hex())
		}
		domain, unpackErr := swapAdapterBinding.UnpackEip712Domain(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("read swap adapter %s domain: %w", addresses[i].Hex(), unpackErr)
		}
		if domain.Fields != [1]byte{0x0f} || domain.Name == "" || domain.Version == "" ||
			domain.ChainId == nil || !domain.ChainId.IsInt64() || domain.ChainId.Int64() != r.chainID ||
			domain.VerifyingContract != addresses[i] || domain.Salt != ([32]byte{}) || len(domain.Extensions) != 0 {
			return nil, errors.Errorf("swap adapter %s has an unsupported EIP-712 domain", addresses[i].Hex())
		}
		domains[addresses[i]] = swapDomain{
			Name: domain.Name, Version: domain.Version, ChainID: new(big.Int).Set(domain.ChainId),
			VerifyingContract: domain.VerifyingContract,
		}
	}
	return domains, nil
}

func (r *swapOnchainReader) readFillQuote(
	ctx context.Context,
	route liquidlane.Route,
	amountIn *big.Int,
) (liquidlane.FillQuote, error) {
	if route.ID == "" || route.Adapter == (common.Address{}) || amountIn == nil || amountIn.Sign() <= 0 {
		return liquidlane.FillQuote{}, errors.New("invalid exact swap route read")
	}
	quotes, err := r.ll.ReadFillQuotes(ctx, []liquidlane.Route{route}, route.TokenIn, amountIn)
	if err != nil {
		return liquidlane.FillQuote{}, errors.Errorf("read exact swap route: %w", err)
	}
	if len(quotes) != 1 || quotes[0].Route != route || quotes[0].AmountIn == nil ||
		quotes[0].AmountIn.Cmp(amountIn) != 0 {
		return liquidlane.FillQuote{}, errors.New("exact swap route is unavailable or changed")
	}
	return cloneFillQuote(quotes[0]), nil
}

func (r *swapOnchainReader) readUsedNonces(ctx context.Context, checks []swapNonceCheck) ([]bool, error) {
	if len(checks) == 0 {
		return []bool{}, nil
	}
	calls := make([]chain.Call, len(checks))
	for i, check := range checks {
		if check.Adapter == (common.Address{}) || check.TokenIn == (common.Address{}) || !validUint256(check.Nonce) {
			return nil, errors.Errorf("invalid swap nonce check %d", i)
		}
		data, err := swapAdapterBinding.TryPackIsUsedNonce(check.TokenIn, check.Nonce)
		if err != nil {
			return nil, errors.Errorf("pack swap nonce check %d: %w", i, err)
		}
		calls[i] = chain.Call{Target: check.Adapter, AllowFailure: true, Data: data}
	}
	results, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, errors.Errorf("read swap nonces: %w", err)
	}
	if len(results) != len(calls) {
		return nil, errors.Errorf("read swap nonces: got %d results, want %d", len(results), len(calls))
	}
	used := make([]bool, len(results))
	for i, result := range results {
		if !result.Success {
			return nil, errors.Errorf("read swap nonce %d: call failed", i)
		}
		value, unpackErr := swapAdapterBinding.UnpackIsUsedNonce(result.ReturnData)
		if unpackErr != nil {
			return nil, errors.Errorf("read swap nonce %d: %w", i, unpackErr)
		}
		used[i] = value
	}
	return used, nil
}

func dedupeSwapAddresses(addresses []common.Address) []common.Address {
	seen := make(map[common.Address]bool, len(addresses))
	out := make([]common.Address, 0, len(addresses))
	for _, address := range addresses {
		if address == (common.Address{}) || seen[address] {
			continue
		}
		seen[address] = true
		out = append(out, address)
	}
	return out
}

func cloneFillQuote(quote liquidlane.FillQuote) liquidlane.FillQuote {
	out := quote
	out.MaxAssets = liquidlane.CloneBig(quote.MaxAssets)
	out.MaxRate = liquidlane.CloneBig(quote.MaxRate)
	out.AdapterMinDiscount = liquidlane.CloneBig(quote.AdapterMinDiscount)
	out.DiscountID = liquidlane.CloneHash(quote.DiscountID)
	out.AmountIn = liquidlane.CloneBig(quote.AmountIn)
	out.GrossAmountOut = liquidlane.CloneBig(quote.GrossAmountOut)
	out.MaxAmountOut = liquidlane.CloneBig(quote.MaxAmountOut)
	out.MinDiscount = liquidlane.CloneBig(quote.MinDiscount)
	return out
}
