package uniswapx

import (
	"context"
	"math/big"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	dexbinding "github.com/symbioticfi/vault-solver/api/bindings/uniswapx/dex"
)

var dexABI = dexbinding.NewUniswapXDex()

type dexQuote struct {
	Amount    *big.Int
	BlockTime time.Time
}

type dexQuoter interface {
	quote(ctx context.Context, token common.Address, amount *big.Int, exactOutput bool) (dexQuote, error)
	validate(ctx context.Context, executor common.Address) error
}

type dexBackend interface {
	contractCaller
	HeaderByNumber(ctx context.Context, number *big.Int) (*gethtypes.Header, error)
}

type v3Dex struct {
	backend dexBackend
	cfg     *DexConfig
	reactor common.Address
	slots   chan struct{}
}

func newV3Dex(backend dexBackend, cfg *DexConfig, reactor common.Address) *v3Dex {
	return &v3Dex{backend: backend, cfg: cfg, reactor: reactor, slots: make(chan struct{}, cfg.MaxConcurrentQuotes)}
}

func (d *v3Dex) call(ctx context.Context, target common.Address, data []byte, block *big.Int) ([]byte, error) {
	return d.backend.CallContract(ctx, ethereum.CallMsg{To: &target, Data: data}, block)
}

func (d *v3Dex) validate(ctx context.Context, executor common.Address) error {
	if d.cfg.Treasury == executor {
		return errors.New("dex treasury must differ from executor")
	}
	for _, field := range []struct {
		data []byte
		want common.Address
	}{
		{dexABI.PackINPUTTOKEN(), d.cfg.InputToken}, {dexABI.PackBRIDGETOKEN(), d.cfg.BridgeToken},
		{dexABI.PackReactorAddress(), d.reactor},
		{dexABI.PackROUTER(), d.cfg.Router}, {dexABI.PackTREASURY(), d.cfg.Treasury},
		{dexABI.PackOUTPUT0(), d.cfg.Outputs[0].Token}, {dexABI.PackOUTPUT1(), d.cfg.Outputs[1].Token},
	} {
		data, err := d.call(ctx, executor, field.data, nil)
		if err != nil || len(data) != 32 || common.BytesToAddress(data) != field.want {
			return errors.New("dex executor immutable configuration does not match")
		}
	}
	for i, data := range [][]byte{dexABI.PackFEE0(), dexABI.PackFEE1()} {
		result, err := d.call(ctx, executor, data, nil)
		if err != nil || len(result) != 32 || new(big.Int).SetBytes(result).Cmp(new(big.Int).SetUint64(uint64(d.cfg.Outputs[i].Fee))) != 0 {
			return errors.New("dex executor pool fee configuration does not match")
		}
	}
	for _, target := range []common.Address{d.cfg.Router, d.cfg.Quoter} {
		data, err := d.call(ctx, target, dexABI.PackFactory(), nil)
		if err != nil || len(data) != 32 || common.BytesToAddress(data) != d.cfg.Factory {
			return errors.New("dex router/quoter factory does not match")
		}
	}
	for _, out := range d.cfg.Outputs {
		data, err := d.call(ctx, d.cfg.Factory,
			dexABI.PackGetPool(d.cfg.BridgeToken, out.Token, new(big.Int).SetUint64(uint64(out.Fee))), nil)
		if err != nil || len(data) != 32 || common.BytesToAddress(data) == (common.Address{}) {
			return errors.New("dex configured pool unavailable at startup")
		}
	}
	return nil
}

// quote pins the fee policy, pool existence and amount-specific quote to one canonical header.
// Quoter is invoked through eth_call; no send/broadcast method exists on this reader.
func (d *v3Dex) quote(ctx context.Context, token common.Address, amount *big.Int, exactOutput bool) (dexQuote, error) {
	ctx, cancel := context.WithTimeout(ctx, d.cfg.RPCTimeout)
	defer cancel()
	select {
	case d.slots <- struct{}{}:
		defer func() { <-d.slots }()
	default:
		return dexQuote{}, errors.New("dex quote concurrency limit reached")
	}
	output, ok := d.cfg.output(token)
	if !ok || amount == nil || amount.Sign() <= 0 || amount.BitLen() > 256 {
		return dexQuote{}, errors.New("invalid dex quote pair or amount")
	}
	header, err := d.backend.HeaderByNumber(ctx, nil)
	if err != nil || header == nil || header.Number == nil || header.Number.Sign() <= 0 {
		return dexQuote{}, errors.New("dex quote header unavailable")
	}
	feePolicy, err := d.call(ctx, d.reactor, dexABI.PackFeeController(), header.Number)
	if err != nil || len(feePolicy) != 32 || common.BytesToAddress(feePolicy) != (common.Address{}) {
		return dexQuote{}, errors.New("dex prototype requires zero Reactor fee controller")
	}
	poolData := dexABI.PackGetPool(d.cfg.BridgeToken, token, new(big.Int).SetUint64(uint64(output.Fee)))
	pool, err := d.call(ctx, d.cfg.Factory, poolData, header.Number)
	if err != nil || len(pool) != 32 || common.BytesToAddress(pool) == (common.Address{}) {
		return dexQuote{}, errors.New("configured dex pool unavailable")
	}
	var data []byte
	if exactOutput {
		data, err = dexABI.TryPackQuoteExactOutputSingle(dexbinding.IUniswapXV3QuoterQuoteExactOutputSingleParams{
			TokenIn: d.cfg.BridgeToken, TokenOut: token, Amount: amount,
			Fee: new(big.Int).SetUint64(uint64(output.Fee)), SqrtPriceLimitX96: new(big.Int),
		})
	} else {
		data, err = dexABI.TryPackQuoteExactInputSingle(dexbinding.IUniswapXV3QuoterQuoteExactInputSingleParams{
			TokenIn: d.cfg.BridgeToken, TokenOut: token, AmountIn: amount,
			Fee: new(big.Int).SetUint64(uint64(output.Fee)), SqrtPriceLimitX96: new(big.Int),
		})
	}
	if err != nil {
		return dexQuote{}, err
	}
	result, err := d.call(ctx, d.cfg.Quoter, data, header.Number)
	if err != nil {
		return dexQuote{}, errors.Errorf("dex quote: %w", err)
	}
	var value *big.Int
	if exactOutput {
		quote, decodeErr := dexABI.UnpackQuoteExactOutputSingle(result)
		value, err = quote.AmountIn, decodeErr
	} else {
		quote, decodeErr := dexABI.UnpackQuoteExactInputSingle(result)
		value, err = quote.AmountOut, decodeErr
	}
	if err != nil || value == nil || value.Sign() <= 0 || value.BitLen() > 256 {
		return dexQuote{}, errors.New("dex quote returned invalid amount")
	}
	canonical, err := d.backend.HeaderByNumber(ctx, header.Number)
	if err != nil || canonical == nil || canonical.Hash() != header.Hash() {
		return dexQuote{}, errors.New("dex quote block changed during calculation")
	}
	return dexQuote{Amount: value, BlockTime: time.Unix(int64(header.Time), 0)}, nil
}
