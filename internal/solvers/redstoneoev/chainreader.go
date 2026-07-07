package redstoneoev

import (
	"context"
	"maps"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/bindings/erc4626"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/aggregator"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/callback"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/executor"
	morphobinding "github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/api/bindings/oev/oracle"
	"github.com/symbioticfi/vault-solver/api/bindings/vaultv2"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/morpho"
)

// Contract binding instances (abigen --v2): typed Pack/Unpack helpers for the Multicall3 sub-calls below,
// driven the same way as the rfq reader, so an ABI change fails at compile time. The LiquidLane adapter is
// a neutral group driven by both rfq and redstone-oev; the ERC-4626 vault binding covers the ERC-20 reads
// (asset(), balanceOf()) — IERC4626 is an ERC-20.
var (
	morphoB     = morphobinding.NewMorpho()
	executorB   = executor.NewRedStoneExecutor()
	aggregatorB = aggregator.NewAggregatorV3()
	callbackB   = callback.NewSymbioticOevSolver()
	oracleB     = oracle.NewMorphoOracle()

	llAdapter = adapter.NewLiquidLaneAdapter()
	erc4626b  = erc4626.NewIERC4626() // asset() + balanceOf() (IERC4626 is an ERC-20)
	vaultV2B  = vaultv2.NewIVaultV2()
)

// abiMarketParams is Morpho's MarketParams tuple (loanToken, collateralToken, oracle, irm, lltv).
type abiMarketParams struct {
	LoanToken       common.Address
	CollateralToken common.Address
	Oracle          common.Address
	Irm             common.Address
	Lltv            *big.Int
}

/* ───────── decoders ───────── */

// decodeMarketParams decodes Morpho idToMarketParams(id) into the params tuple.
func decodeMarketParams(data []byte) (abiMarketParams, error) {
	out, err := morphoB.UnpackIdToMarketParams(data)
	if err != nil {
		return abiMarketParams{}, errors.Errorf("decode marketParams: %w", err)
	}
	if out.Lltv == nil {
		return abiMarketParams{}, errors.New("decode marketParams: lltv nil")
	}
	return abiMarketParams{
		LoanToken: out.LoanToken, CollateralToken: out.CollateralToken,
		Oracle: out.Oracle, Irm: out.Irm, Lltv: out.Lltv,
	}, nil
}

func decodeLatestRoundData(data []byte) (answer, updatedAt *big.Int, err error) {
	out, e := aggregatorB.UnpackLatestRoundData(data)
	if e != nil {
		return nil, nil, errors.Errorf("decode latestRoundData: %w", e)
	}
	if out.Answer == nil || out.UpdatedAt == nil {
		return nil, nil, errors.New("decode latestRoundData: nil field")
	}
	return out.Answer, out.UpdatedAt, nil
}

func decodeDecimals(data []byte) (uint8, error) {
	d, err := aggregatorB.UnpackDecimals(data)
	if err != nil {
		return 0, errors.Errorf("decode decimals: %w", err)
	}
	return d, nil
}

/* ───────── market id re-derivation ───────── */

// marketParamsArgs is the ABI tuple of Morpho MarketParams, used to recompute a market id. It encodes
// the exact (address,address,address,address,uint256) tuple Morpho's Id library hashes — kept hand-built
// (vs the getter ABI, whose outputs are flattened, not a bare abi.encode of a tuple) so deriveMarketID
// stays byte-exact with the on-chain id (pinned in marketid_test.go).
var marketParamsArgs = abi.Arguments{{Type: mustTupleType([]abi.ArgumentMarshaling{
	{Name: "loanToken", Type: "address"},
	{Name: "collateralToken", Type: "address"},
	{Name: "oracle", Type: "address"},
	{Name: "irm", Type: "address"},
	{Name: "lltv", Type: "uint256"},
})}}

func mustTupleType(components []abi.ArgumentMarshaling) abi.Type {
	t, err := abi.NewType("tuple", "", components)
	if err != nil {
		panic("redstoneoev: market params tuple type: " + err.Error())
	}
	return t
}

// deriveMarketID recomputes a Morpho market id = keccak256(abi.encode(MarketParams)), used to verify a
// resolved id against the params Morpho returned for it — a spoofed or non-existent id (Morpho returns
// zero params) re-derives to a different hash and is dropped (fail closed).
func deriveMarketID(p abiMarketParams) (common.Hash, error) {
	enc, err := marketParamsArgs.Pack(p)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode market params: %w", err)
	}
	return crypto.Keccak256Hash(enc), nil
}

const maxFeedDecimals = 36

// reader performs the OEV on-chain reads, batching via Multicall3. Nothing here runs on the hot path.
type reader struct {
	chain    *chain.Client
	log      logr.Logger
	decimals *chain.Decimals
	// mu guards the two adapter caches below: both are read from the run-loop and discovery goroutines
	// (refreshMarkets / discoverMarkets), so the map access is locked — the RPC resolve runs unlocked, only
	// the cache read/write takes mu.
	mu          sync.Mutex
	adapterLoan map[common.Address]common.Address   // adapter.vault().asset(), resolved once (immutable)
	redeemColl  map[common.Address][]common.Address // adapter's redeemable collateral set, resolved once (stable)
}

func feedDecimalsInBounds(loanDec, ethDec uint8) bool {
	return loanDec <= maxFeedDecimals && ethDec <= maxFeedDecimals
}

func feedFresh(updatedAt, nowSec, maxAge int64) bool {
	age := nowSec - updatedAt
	return age >= 0 && age <= maxAge
}

func newReader(c *chain.Client, log logr.Logger) *reader {
	return &reader{
		chain: c, log: log,
		decimals:    chain.NewDecimals(c),
		adapterLoan: map[common.Address]common.Address{},
		redeemColl:  map[common.Address][]common.Address{},
	}
}

// adapterLoanToken returns the adapter's vault loan token (adapter.vault().asset()), caching the result
// (immutable). Up to two multicalls when uncached: vault(), then asset() on the vault. Returns the zero
// address (and ok=false) when either read fails — the market then resolves to no quote (fail closed).
func (r *reader) adapterLoanToken(ctx context.Context, adapter common.Address) (common.Address, bool, error) {
	r.mu.Lock()
	lt, ok := r.adapterLoan[adapter]
	r.mu.Unlock()
	if ok {
		return lt, true, nil
	}
	vault, err := r.callAddress(ctx, adapter, llAdapter.PackVault(), llAdapter.UnpackVault)
	if err != nil {
		return common.Address{}, false, err
	}
	if vault == (common.Address{}) {
		return common.Address{}, false, nil // vault() reverted / didn't decode → fail closed
	}
	asset, err := r.callAddress(ctx, vault, erc4626b.PackAsset(), erc4626b.UnpackAsset)
	if err != nil {
		return common.Address{}, false, err
	}
	if asset == (common.Address{}) {
		return common.Address{}, false, nil // asset() reverted / didn't decode → fail closed
	}
	r.mu.Lock()
	r.adapterLoan[adapter] = asset
	r.mu.Unlock()
	return asset, true, nil
}

type adapterSnapshot struct {
	loan       common.Address
	redeemable []common.Address
	filler     bool
}

func (r *reader) readAdapterSnapshot(ctx context.Context, callback, adapter common.Address) (adapterSnapshot, error) {
	loan, ok, err := r.adapterLoanToken(ctx, adapter)
	if err != nil {
		return adapterSnapshot{}, errors.Errorf("adapter loan token: %w", err)
	}
	if !ok || loan == (common.Address{}) {
		return adapterSnapshot{}, errors.New("adapter loan token unresolved")
	}
	redeemable, err := r.readRedeemableCollaterals(ctx, adapter)
	if err != nil {
		return adapterSnapshot{}, errors.Errorf("adapter redeemable collateral: %w", err)
	}
	if len(redeemable) == 0 {
		return adapterSnapshot{}, errors.New("adapter redeemable collateral unresolved")
	}
	filler, err := r.ReadFillerStatus(ctx, callback, adapter)
	if err != nil {
		return adapterSnapshot{}, errors.Errorf("adapter filler status: %w", err)
	}
	return adapterSnapshot{loan: loan, redeemable: redeemable, filler: filler}, nil
}

// callAddress reads a single address-returning view method off `target` in one multicall (the call packed
// by `data`, the return decoded by `unpack` — the binding's typed PackXxx/UnpackXxx), returning the zero
// address (not an error) when the sub-call reverts or doesn't decode — only an RPC failure surfaces as an
// error. So a zero-address result means "fail closed" to the caller.
func (r *reader) callAddress(ctx context.Context, target common.Address, data []byte, unpack func([]byte) (common.Address, error)) (common.Address, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: target, AllowFailure: true, Data: data},
	})
	if err != nil {
		return common.Address{}, err
	}
	if len(res) != 1 || !res[0].Success {
		return common.Address{}, nil // sub-call reverted → fail closed (zero address)
	}
	out, derr := unpack(res[0].ReturnData)
	if derr != nil {
		out = common.Address{} // didn't decode → fail closed (zero address)
	}
	return out, nil
}

// ReadLoanEthRate composes live loanPerEth from loan/USD and ETH/USD feeds. Nil means no usable feed value.
func (r *reader) ReadLoanEthRate(ctx context.Context, adapter common.Address, feed *loanEthFeed, now time.Time) *big.Int {
	if feed == nil {
		return nil
	}
	token, ok, err := r.adapterLoanToken(ctx, adapter)
	if err != nil || !ok {
		return nil
	}
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: feed.LoanUsdFeed, AllowFailure: true, Data: aggregatorB.PackLatestRoundData()},
		{Target: feed.LoanUsdFeed, AllowFailure: true, Data: aggregatorB.PackDecimals()},
		{Target: feed.EthUsdFeed, AllowFailure: true, Data: aggregatorB.PackLatestRoundData()},
		{Target: feed.EthUsdFeed, AllowFailure: true, Data: aggregatorB.PackDecimals()},
	})
	if err != nil || !allSuccess(res, 4) {
		return nil
	}
	loanAns, loanUp, e1 := decodeLatestRoundData(res[0].ReturnData)
	loanDecFeed, e2 := decodeDecimals(res[1].ReturnData)
	ethAns, ethUp, e3 := decodeLatestRoundData(res[2].ReturnData)
	ethDecFeed, e4 := decodeDecimals(res[3].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
		return nil
	}
	if !feedDecimalsInBounds(loanDecFeed, ethDecFeed) {
		r.log.Error(errors.New("feed decimals out of bounds"),
			"loanPerEth feed rejected", "loanFeedDec", loanDecFeed, "ethFeedDec", ethDecFeed, "max", maxFeedDecimals)
		return nil
	}
	nowSec, maxAge := now.Unix(), int64((feed.MaxAge+time.Second-1)/time.Second)
	if !feedFresh(loanUp.Int64(), nowSec, maxAge) || !feedFresh(ethUp.Int64(), nowSec, maxAge) {
		r.log.V(1).Info("loan/ETH rate feeds stale",
			"loanFeed", feed.LoanUsdFeed.Hex(), "loanAgeSec", nowSec-loanUp.Int64(),
			"ethFeed", feed.EthUsdFeed.Hex(), "ethAgeSec", nowSec-ethUp.Int64(),
			"maxAgeSec", maxAge)
		return nil
	}
	loanDec, e := r.decimals.Get(ctx, token)
	if e != nil {
		return nil
	}
	return composeLoanPerEth(ethAns, loanAns, int(ethDecFeed), int(loanDecFeed), loanDec)
}

// readRedeemableCollaterals returns the adapter's redeemable collateral SET (the markets its loan token
// can liquidate into): getTokensToRedeemLength() then tokensToRedeem(0..n-1) batched in one multicall on
// the adapter. The set is stable, so the result is cached (mirrors the adapterLoan immutable cache). Fails
// CLOSED — returns nil (no discovery this round) when the length read reverts/doesn't decode or any
// tokensToRedeem entry fails — so a partial read never narrows market discovery to a wrong subset.
func (r *reader) readRedeemableCollaterals(ctx context.Context, adapter common.Address) ([]common.Address, error) {
	r.mu.Lock()
	c, ok := r.redeemColl[adapter]
	r.mu.Unlock()
	if ok {
		return slices.Clone(c), nil
	}
	lenRes, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackGetTokensToRedeemLength()},
	})
	if err != nil {
		return nil, err
	}
	count, ok := decodeRedeemCount(lenRes)
	if !ok {
		return nil, nil // length read reverted / didn't decode → fail closed (no discovery)
	}
	if count == 0 {
		r.mu.Lock()
		r.redeemColl[adapter] = nil // an empty set is a valid (cached) answer
		r.mu.Unlock()
		return nil, nil
	}
	calls := make([]chain.Call, count)
	for i := range count {
		calls[i] = chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackTokensToRedeem(big.NewInt(int64(i)))}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	toks, ok := decodeRedeemTokens(res, count)
	if !ok {
		return nil, nil // a partial/undecodable read → fail closed (don't cache a wrong subset)
	}
	r.mu.Lock()
	r.redeemColl[adapter] = slices.Clone(toks)
	r.mu.Unlock()
	return toks, nil
}

// decodeRedeemCount decodes getTokensToRedeemLength() from its single-call result into a non-negative
// int64-bounded count, returning ok=false on a reverted/undecodable/absurd length (caller fails closed).
// Pure → unit-testable.
func decodeRedeemCount(res []chain.CallResult) (int, bool) {
	if len(res) != 1 || !res[0].Success {
		return 0, false
	}
	n, err := llAdapter.UnpackGetTokensToRedeemLength(res[0].ReturnData)
	if err != nil || n == nil || n.Sign() < 0 || !n.IsInt64() {
		return 0, false
	}
	return int(n.Int64()), true
}

// decodeRedeemTokens decodes the tokensToRedeem(i) multicall results into the collateral set, returning
// ok=false if any sub-call failed/didn't decode or yielded the zero address — the caller then fails closed.
// Pure (no I/O) so the strided decode is unit-testable against hand-packed CallResults.
func decodeRedeemTokens(res []chain.CallResult, count int) ([]common.Address, bool) {
	if len(res) != count {
		return nil, false
	}
	out := make([]common.Address, 0, count)
	for i := range res {
		if !res[i].Success {
			return nil, false
		}
		tok, err := llAdapter.UnpackTokensToRedeem(res[i].ReturnData)
		if err != nil || tok == (common.Address{}) {
			return nil, false
		}
		out = append(out, tok)
	}
	return out, true
}

// verifyAdapterPair filters resolved market params to those the adapter can actually liquidate: loan token
// == the adapter's loan AND collateral ∈ the adapter's redeemable set. params come from ResolveParams (each
// already keccak-verified against its id), so this is the pair half of the on-chain verification. Pure →
// unit-testable.
func verifyAdapterPair(params map[common.Hash]abiMarketParams, adapterLoan common.Address, redeemable []common.Address) []common.Hash {
	redeem := make(map[common.Address]bool, len(redeemable))
	for _, t := range redeemable {
		redeem[t] = true
	}
	out := make([]common.Hash, 0, len(params))
	for id, p := range params {
		if p.LoanToken == adapterLoan && redeem[p.CollateralToken] {
			out = append(out, id)
		}
	}
	return out
}

// MarketInfo is a market's params plus its API snapshot state. The serving adapter is NOT here: it is the
// solver's single configured adapter (cfg.Adapter), and its redemption quote travels separately in snapshot.
type MarketInfo struct {
	Params abiMarketParams
	State  morpho.MarketState
}

// ResolveParams reads idToMarketParams for each id in ONE multicall and returns the immutable market
// params, keyed by id. Each id is verified by re-deriving keccak256(abi.encode(params)) and dropping
// any mismatch — so a non-existent / spoofed id (Morpho returns zero params) fails closed. Params are
// immutable per id, so the monitor caches the result and only calls this for not-yet-seen ids.
func (r *reader) ResolveParams(ctx context.Context, morpho common.Address, ids []common.Hash) (map[common.Hash]abiMarketParams, error) {
	if len(ids) == 0 {
		return map[common.Hash]abiMarketParams{}, nil
	}
	calls := make([]chain.Call, len(ids))
	for i, id := range ids {
		calls[i] = chain.Call{Target: morpho, AllowFailure: true, Data: morphoB.PackIdToMarketParams(id)}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("resolveParams: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]abiMarketParams, len(ids))
	for i, id := range ids {
		if !res[i].Success {
			continue
		}
		mp, derr := decodeMarketParams(res[i].ReturnData)
		if derr != nil {
			continue
		}
		if derived, verr := deriveMarketID(mp); verr != nil || derived != id {
			r.log.V(1).Info("market id mismatch; dropping", "id", id.Hex()) // fail closed (unknown/spoofed id)
			continue
		}
		out[id] = mp
	}
	return out, nil
}

// ReadAdapterQuotes reads only the single configured LiquidLane adapter's quote for served markets. API
// mode uses this while Morpho market state and positions come from the indexer snapshot.
func (r *reader) ReadAdapterQuotes(ctx context.Context, params map[common.Hash]abiMarketParams, adapter common.Address, serve map[common.Hash]bool) (map[common.Hash]*AdapterQuote, error) {
	if len(params) == 0 {
		return map[common.Hash]*AdapterQuote{}, nil
	}
	ids := slices.SortedFunc(maps.Keys(params), common.Hash.Cmp)
	tokens := make([]common.Address, 0, len(ids)*2)
	for _, id := range ids {
		p := params[id]
		tokens = append(tokens, p.LoanToken, p.CollateralToken)
	}
	decs, err := r.decimals.GetMany(ctx, tokens)
	if err != nil {
		return nil, err
	}

	type slot struct {
		id common.Hash
		at int
	}
	var slots []slot
	var calls []chain.Call
	for _, id := range ids {
		if !serve[id] {
			continue
		}
		p := params[id]
		slots = append(slots, slot{id: id, at: len(calls)})
		calls = append(calls,
			chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackGetMaxRate(p.CollateralToken)},
			chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackGetMaxAssets(p.CollateralToken)},
			chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackPaused()},
		)
	}
	if len(calls) == 0 {
		return map[common.Hash]*AdapterQuote{}, nil
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if len(res) != len(calls) {
		return nil, errors.Errorf("adapterQuotes: got %d results, want %d", len(res), len(calls))
	}
	out := make(map[common.Hash]*AdapterQuote, len(slots))
	for _, s := range slots {
		p := params[s.id]
		out[s.id] = buildQuote(res[s.at], res[s.at+1], res[s.at+2], decs, p.LoanToken, p.CollateralToken)
	}
	return out, nil
}

// buildQuote assembles the adapter redemption quote from the three adapter sub-call results, returning
// nil (no biddable exit) when paused, missing either token's decimals, or a non-positive rate/liquidity.
// An UNREADABLE pause state — paused() reverted or didn't decode — is treated as PAUSED (fail closed): we
// must not bid a leg whose adapter might be paused (the swap would revert), mirroring every other guard here.
func buildQuote(rateRes, maxAssetsRes, pausedRes chain.CallResult, decs map[common.Address]int, loanTok, collTok common.Address) *AdapterQuote {
	loanDec, okLoan := decs[loanTok]
	collDec, okColl := decs[collTok]
	if !okLoan || !okColl {
		return nil
	}
	if !pausedRes.Success {
		return nil // unknown pause state ⇒ treat as paused (fail closed)
	}
	p, perr := llAdapter.UnpackPaused(pausedRes.ReturnData)
	if perr != nil || p {
		return nil // undecodable ⇒ fail closed; explicitly paused ⇒ no quote
	}
	if !rateRes.Success || !maxAssetsRes.Success {
		return nil
	}
	rate, e1 := llAdapter.UnpackGetMaxRate(rateRes.ReturnData)
	maxAssets, e2 := llAdapter.UnpackGetMaxAssets(maxAssetsRes.ReturnData)
	if e1 != nil || e2 != nil || rate.Sign() <= 0 || maxAssets.Sign() <= 0 {
		return nil
	}
	return &AdapterQuote{
		MaxRate: rate, MaxAssets: maxAssets,
		LoanScale: chain.Exp10(loanDec), CollScale: chain.Exp10(collDec), // precompute for the hot path
	}
}

// ExecutorState is the signer's accounting on the RedStone Executor.
type ExecutorState struct {
	Nonce   *big.Int
	Deposit *big.Int
	Locked  bool
}

// ReadExecutorState reads nonces/deposits/locked for the signer in one multicall.
func (r *reader) ReadExecutorState(ctx context.Context, executor, signer common.Address) (ExecutorState, error) {
	res, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: executor, AllowFailure: true, Data: executorB.PackNonces(signer)},
		{Target: executor, AllowFailure: true, Data: executorB.PackDeposits(signer)},
		{Target: executor, AllowFailure: true, Data: executorB.PackLocked(signer)},
	})
	if err != nil {
		return ExecutorState{}, err
	}
	if !allSuccess(res, 3) {
		return ExecutorState{}, errors.New("executor state read reverted")
	}
	nonce, e1 := executorB.UnpackNonces(res[0].ReturnData)
	deposit, e2 := executorB.UnpackDeposits(res[1].ReturnData)
	locked, e3 := executorB.UnpackLocked(res[2].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil {
		return ExecutorState{}, errors.New("executor state decode failed")
	}
	return ExecutorState{Nonce: nonce, Deposit: deposit, Locked: locked}, nil
}

// ReadGasPredictorState caches the LiquidLane/vault balances needed to classify each selected leg's route.
func (r *reader) ReadGasPredictorState(ctx context.Context, adapter common.Address, collaterals []common.Address) (*gasPredictorState, error) {
	colls := dedupeAddresses(collaterals)
	if len(colls) == 0 {
		return nil, nil
	}
	head, err := r.chain.Multicall(ctx, []chain.Call{
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackOwner()},
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackMarketMaker()},
		{Target: adapter, AllowFailure: true, Data: llAdapter.PackVault()},
	})
	if err != nil {
		return nil, err
	}
	if !allSuccess(head, 3) {
		return nil, errors.New("gas predictor head read reverted")
	}
	owner, e1 := llAdapter.UnpackOwner(head[0].ReturnData)
	marketMaker, e2 := llAdapter.UnpackMarketMaker(head[1].ReturnData)
	vault, e3 := llAdapter.UnpackVault(head[2].ReturnData)
	if e1 != nil || e2 != nil || e3 != nil || vault == (common.Address{}) {
		return nil, errors.New("gas predictor head decode failed")
	}

	calls := []chain.Call{
		{Target: vault, AllowFailure: true, Data: vaultV2B.PackFreeAssets()},
		{Target: vault, AllowFailure: true, Data: vaultV2B.PackWithdrawable()},
	}
	for _, coll := range colls {
		calls = append(calls, chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(coll, owner)})
		if marketMaker != owner {
			calls = append(calls, chain.Call{Target: adapter, AllowFailure: true, Data: llAdapter.PackAcquireBalance(coll, marketMaker)})
		}
	}
	res, err := r.chain.Multicall(ctx, calls)
	if err != nil {
		return nil, err
	}
	if !allSuccess(res, len(calls)) {
		return nil, errors.New("gas predictor state read reverted")
	}
	free, e1 := vaultV2B.UnpackFreeAssets(res[0].ReturnData)
	withdrawable, e2 := vaultV2B.UnpackWithdrawable(res[1].ReturnData)
	if e1 != nil || e2 != nil || free == nil || withdrawable == nil {
		return nil, errors.New("gas predictor vault decode failed")
	}
	st := &gasPredictorState{
		FreeAssets:   free,
		Withdrawable: withdrawable,
		Acquire:      make(map[common.Address]*big.Int, len(colls)),
	}
	idx := 2
	for _, coll := range colls {
		ownerBal, derr := llAdapter.UnpackAcquireBalance(res[idx].ReturnData)
		idx++
		if derr != nil || ownerBal == nil {
			return nil, errors.New("gas predictor acquire decode failed")
		}
		total := new(big.Int).Set(ownerBal)
		if marketMaker != owner {
			mmBal, merr := llAdapter.UnpackAcquireBalance(res[idx].ReturnData)
			idx++
			if merr != nil || mmBal == nil {
				return nil, errors.New("gas predictor market-maker acquire decode failed")
			}
			total.Add(total, mmBal)
		}
		st.Acquire[coll] = total
	}
	return st, nil
}

func dedupeAddresses(in []common.Address) []common.Address {
	seen := make(map[common.Address]bool, len(in))
	out := make([]common.Address, 0, len(in))
	for _, a := range in {
		if a == (common.Address{}) || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	return out
}

// allSuccess reports whether a fixed-shape multicall returned exactly n results and every one succeeded —
// the reverted-guard for single/triple-call view reads before decoding.
func allSuccess(res []chain.CallResult, n int) bool {
	if len(res) != n {
		return false
	}
	for i := range res {
		if !res[i].Success {
			return false
		}
	}
	return true
}
