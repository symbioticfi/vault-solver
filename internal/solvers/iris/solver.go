// Package iris answers IRIS RFQ quote requests for one Merkle account. The account is the
// Iris solver: its offer recipes, one per market, presign each quote on-chain (ERC-1271)
// before the quote is returned. See docs/IRIS-PLAN.md.
package iris

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/merkle-executor/sdk/transaction"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/api/bindings/oev/morpho"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/solver"
)

const (
	Name = "iris"
	// maxConfirmations keeps the receipt within one slot of inclusion; deeper confirmation
	// would date every deadline too early for the coordinator.
	maxConfirmations = 1
	shutdownTimeout  = 5 * time.Second
)

var (
	executorBinding = recipeexecutor.NewRecipeExecutor()
	morphoBinding   = morpho.NewMorpho()
	emptyDataHash   = crypto.Keccak256Hash(nil)
)

var tracer = observability.NewTracer("github.com/symbioticfi/vault-solver/internal/solvers/iris", Name)

//nolint:gochecknoinits // Self-registration is the solver plugin contract.
func init() { solver.Register(Name, factory) }

// chainReader is the chain surface the solver reads; *chain.Client satisfies it.
type chainReader interface {
	recipes.Reader
}

// sender is the shared Merkle transaction manager's non-strict surface.
type sender interface {
	LaneReady() bool
	SendAsync(ctx context.Context, req transaction.Request) (<-chan transaction.Result, bool)
}

// Solver serves the coordinator webhook. The market index is replaced whole by the refresh
// loop and read lock-free; firm quotes are signed one at a time.
type Solver struct {
	cfg       Config
	chain     chainReader
	chainID   *big.Int
	signer    common.Address
	maxGas    uint64
	source    *permissions.Source
	sender    sender
	authValue string
	metrics   *metrics

	markets atomic.Pointer[marketIndex]
	// built holds the on-chain words the index was decoded from; only the refresh loop uses it.
	built []common.Hash

	// firm admits one firm quote at a time; reservations hold the bonds of signed quotes.
	firm         sync.Mutex
	reservedMu   sync.Mutex
	reservations []reservation
}

func (*Solver) Name() string                  { return Name }
func (*Solver) RequiresTxManager() bool       { return false }
func (*Solver) RequiresMerkleTxManager() bool { return true }

func factory(raw yaml.Node, deps solver.Deps) (solver.Solver, error) {
	cfg, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	if deps.Chain == nil || deps.Signer == nil || deps.MerkleTxManager == nil {
		return nil, errors.New("iris requires the host chain, signer and Merkle transaction manager")
	}
	source, err := cfg.Curator.PermissionSource(deps.Chain)
	if err != nil {
		return nil, err
	}
	manager, err := deps.MerkleTxManager()
	if err != nil {
		return nil, err
	}
	if manager.Confirmations() > maxConfirmations {
		return nil, errors.Errorf("iris requires txManager.confirmations <= %d", maxConfirmations)
	}
	s := &Solver{
		cfg: cfg, chain: deps.Chain, chainID: deps.Chain.ChainID(), signer: deps.Signer.Address(), maxGas: deps.MaxGas,
		source: source, sender: manager,
		metrics: newMetrics(deps.Metrics),
	}
	if s.authValue = os.Getenv(cfg.Server.AuthValueEnv); s.authValue == "" {
		return nil, errors.Errorf("server.authValueEnv: %s is empty", cfg.Server.AuthValueEnv)
	}
	return s, nil
}

// Run discovers the account's markets, then serves quotes and refreshes the index until ctx ends.
func (s *Solver) Run(ctx context.Context) error {
	if err := s.refresh(ctx); err != nil {
		return errors.Errorf("discover iris markets: %w", err)
	}
	server := s.newServer(ctx)
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", server.Addr)
	if err != nil {
		return errors.Errorf("listen %s: %w", server.Addr, err)
	}
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			return errors.Errorf("serve iris quotes: %w", err)
		}
		return nil
	})
	group.Go(func() error {
		ticker := time.NewTicker(s.cfg.RefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
				defer cancel()
				return server.Shutdown(shutdown)
			case <-ticker.C:
				if err := s.refresh(ctx); err != nil {
					observability.ErrorUnlessCanceled(ctx, err, "iris market refresh failed; keeping previous markets")
				}
			}
		}
	})
	return group.Wait()
}

// refresh rebuilds the market index from the configured executors when their recipe state
// changed. Executors that are paused (zero caller), serve another account, or hold no usable
// offer program are skipped and logged, so the healthy markets stay quotable.
func (s *Solver) refresh(ctx context.Context) (err error) {
	ctx, end := tracer.Start(ctx, "iris.discover")
	defer func() { end(err) }()
	state, err := s.recipeState(ctx)
	if err != nil {
		return err
	}
	if slices.Equal(state, s.built) {
		return nil
	}
	prepared := make(map[common.Address]recipes.PreparedRecipe)
	for caller, executors := range s.callerGroups(ctx, state) {
		accounts := make([]common.Address, len(executors))
		for i := range accounts {
			accounts[i] = s.cfg.Account
		}
		results, err := recipes.DiscoverCandidates(ctx, s.chain, executors, accounts, caller, recipes.PreparationOptions{ChainID: s.chainID})
		if err != nil {
			return err
		}
		for i, result := range results {
			if result.Err != nil {
				s.skip(ctx, executors[i], result.Err)
				continue
			}
			prepared[executors[i]] = result.Prepared
		}
	}
	trees := make(map[common.Hash]*recipes.Tree)
	markets := make([]Market, 0, len(prepared))
	for _, executor := range s.cfg.RecipeExecutors {
		member, found := prepared[executor]
		if !found {
			continue
		}
		market, err := s.admit(ctx, member, trees)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			s.skip(ctx, executor, err)
			continue
		}
		markets = append(markets, market)
	}
	if markets, err = s.withMarketData(ctx, markets); err != nil {
		return err
	}
	if len(markets) == 0 {
		return errors.New("no executor holds a usable offer recipe")
	}
	index := newMarketIndex(markets)
	s.markets.Store(&index)
	s.built = state
	observability.Log(ctx).Info("iris markets discovered", "markets", len(markets))
	return nil
}

// callerGroups groups the executors serving the account by caller, for discovery. state is
// recipeState's words.
func (s *Solver) callerGroups(ctx context.Context, state []common.Hash) map[common.Address][]common.Address {
	groups := make(map[common.Address][]common.Address)
	for i, executor := range s.cfg.RecipeExecutors {
		account := common.BytesToAddress(state[stateWords*i].Bytes())
		caller := common.BytesToAddress(state[stateWords*i+1].Bytes())
		switch {
		case caller == (common.Address{}):
			observability.Log(ctx).Info("iris executor paused", "executor", executor.Hex())
		case account != s.cfg.Account:
			s.skip(ctx, executor, errors.Errorf("executor serves account %s", account.Hex()))
		default:
			groups[caller] = append(groups[caller], executor)
		}
	}
	return groups
}

// admit checks the executor routes the signer and decodes its offer recipe; trees caches the
// permission tree per root.
func (s *Solver) admit(ctx context.Context, member recipes.PreparedRecipe, trees map[common.Hash]*recipes.Tree) (Market, error) {
	snapshot := member.Snapshot
	if _, err := recipes.RouteExecution(ctx, s.chain, snapshot, s.signer, executorBinding.PackExecute(nil, nil, nil, nil)); err != nil {
		return Market{}, errors.Errorf("executor does not admit the signer: %w", err)
	}
	tree, found := trees[snapshot.Root]
	if !found {
		var err error
		if tree, err = s.permissionTree(ctx, snapshot); err != nil {
			return Market{}, err
		}
		trees[snapshot.Root] = tree
	}
	return s.admitMarket(member, tree)
}

// skip logs an executor left out of the index; one unusable executor must not take the
// account's other markets down with it.
func (*Solver) skip(ctx context.Context, executor common.Address, err error) {
	observability.Log(ctx).Error(err, "iris executor skipped", "executor", executor.Hex())
}

// admitMarket decodes an executor's offer recipe and checks the configured timing fits the
// recipe's deadline bound.
func (s *Solver) admitMarket(member recipes.PreparedRecipe, tree *recipes.Tree) (Market, error) {
	market, err := decodeMarket(member, tree, s.cfg.Iris, s.chainID)
	if err != nil {
		return Market{}, err
	}
	// A signature mined a slot before its target dates the deadline one slot further out.
	if lifetime := s.cfg.Timing.QuoteTTL + s.cfg.Timing.ResponseSlack + s.cfg.Timing.BlockTime; lifetime > time.Duration(market.MaxLifetime)*time.Second {
		return Market{}, errors.Errorf("timing needs a %s quote lifetime but the recipe allows %ds", lifetime, market.MaxLifetime)
	}
	return market, nil
}

// withMarketData fills each market's payload, the bytes whose hash the recipe fixes: empty for
// pool venues, otherwise the Morpho market parameters (their hash is the Morpho market id), read
// on-chain in one multicall. A market whose payload cannot be recovered is logged and dropped.
func (s *Solver) withMarketData(ctx context.Context, markets []Market) ([]Market, error) {
	var requests []protocolio.ReadRequest
	var morpho []int
	for i := range markets {
		if markets[i].DataHash == emptyDataHash {
			markets[i].Data = []byte{}
			continue
		}
		if s.cfg.Morpho == (common.Address{}) {
			continue
		}
		requests = append(requests, protocolio.ContractRead(s.cfg.Morpho, morphoBinding.PackIdToMarketParams(markets[i].DataHash), nil, true))
		morpho = append(morpho, i)
	}
	if len(requests) > 0 {
		reads, err := protocolio.ReadGroup(ctx, s.chain, requests...)
		if err != nil {
			return nil, errors.Errorf("read Morpho markets: %w", err)
		}
		for k, i := range morpho {
			raw, err := reads.At(k).CallData()
			if err != nil {
				return nil, errors.Errorf("read Morpho markets: %w", err)
			}
			if params, err := morphoBinding.UnpackIdToMarketParams(raw); err == nil && crypto.Keccak256Hash(raw) == markets[i].DataHash &&
				params.CollateralToken == markets[i].CollateralToken && params.LoanToken == markets[i].DebtToken {
				markets[i].Data = raw
			}
		}
	}
	return slices.DeleteFunc(markets, func(m Market) bool {
		if m.Data != nil {
			return false
		}
		err := errors.Errorf("market payload %s is not a Morpho market for the recipe's tokens (morpho %s)", m.DataHash.Hex(), s.cfg.Morpho.Hex())
		observability.Log(ctx).Error(err, "iris executor skipped", "executor", m.Executor.Hex())
		return true
	}), nil
}

// stateWords is how many recipeState words each executor has.
const stateWords = 4

// recipeState reads, in one batch at latest, everything the SDK compares before admitting a
// prepared recipe: each executor's account, caller, recipe hash and recipe block, then the
// account root. Equal words mean the recipe bodies and the permission tree the index was
// decoded from are unchanged.
func (s *Solver) recipeState(ctx context.Context) ([]common.Hash, error) {
	requests := make([]protocolio.ReadRequest, 0, stateWords*len(s.cfg.RecipeExecutors)+1)
	for _, executor := range s.cfg.RecipeExecutors {
		for _, data := range [][]byte{executorBinding.PackAccount(), executorBinding.PackCaller(), executorBinding.PackRecipeHash(), executorBinding.PackRecipeBlock()} {
			requests = append(requests, protocolio.ContractRead(executor, data, nil, true))
		}
	}
	requests = append(requests, protocolio.ContractRead(s.cfg.Account, accountBinding.PackRoot(), nil, true))
	reads, err := protocolio.ReadGroup(ctx, s.chain, requests...)
	if err != nil {
		return nil, errors.Errorf("read recipe state: %w", err)
	}
	words := make([]common.Hash, len(requests))
	for i := range requests {
		data, err := reads.At(i).CallData()
		if err != nil {
			return nil, errors.Errorf("read recipe state: %w", err)
		}
		if len(data) != common.HashLength {
			return nil, errors.New("read recipe state: malformed word")
		}
		words[i] = common.Hash(data)
	}
	return words, nil
}

func (s *Solver) permissionTree(ctx context.Context, snapshot recipes.Snapshot) (*recipes.Tree, error) {
	raw, err := s.source.PermissionTree(ctx, s.chainID, snapshot)
	if err != nil {
		return nil, errors.Errorf("permission tree: %w", err)
	}
	tree, err := recipes.DecodeTree(raw)
	if err != nil {
		return nil, errors.Errorf("permission tree: %w", err)
	}
	if tree.Root != snapshot.Root || tree.ChainID == nil || tree.ChainID.Cmp(s.chainID) != 0 || tree.Account == nil || *tree.Account != snapshot.Account {
		return nil, errors.New("permission tree scope mismatch")
	}
	return tree, nil
}

func (s *Solver) newServer(ctx context.Context) *http.Server {
	// Handlers inherit the solver logger; cancellation is dropped so Shutdown drains them.
	handlerCtx := context.WithoutCancel(ctx)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /quote", s.handle(false))
	mux.HandleFunc("POST /quote/indicative", s.handle(true))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	timeout := s.cfg.Timing.ResponseBudget + time.Second
	return &http.Server{
		Addr: s.cfg.Server.ListenAddress, Handler: observability.TraceHandler(mux, func(r *http.Request) string { return r.URL.Path }),
		BaseContext:       func(net.Listener) context.Context { return handlerCtx },
		ReadHeaderTimeout: 2 * time.Second, ReadTimeout: timeout, WriteTimeout: timeout, IdleTimeout: 30 * time.Second,
	}
}

func (s *Solver) handle(indicative bool) http.HandlerFunc {
	kind := "firm"
	if indicative {
		kind = "indicative"
	}
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		outcome := "error"
		defer func() { s.metrics.observe(kind, outcome, time.Since(started)) }()
		if s.authValue == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get(s.cfg.Server.AuthHeader)), []byte(s.authValue)) != 1 {
			outcome = "unauthorized"
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body quoteRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBytes)).Decode(&body); err != nil {
			outcome = "invalid_request"
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.Timing.ResponseBudget)
		defer cancel()
		response, err := s.serve(ctx, body, indicative)
		var refused *declined
		switch {
		case err == nil:
			outcome = "quoted"
			writeJSON(ctx, w, http.StatusOK, response)
		case errors.As(err, &refused):
			outcome = refused.reason
			reason := declineReason{Venue: refused.venue, Reason: refused.reason}
			if refused.rate != nil {
				reason.Rate = refused.rate.String()
			}
			writeJSON(ctx, w, http.StatusNotFound, declineBody{Reasons: []declineReason{reason}})
		default:
			observability.ErrorUnlessCanceled(ctx, err, "iris quote failed", "requestId", body.RequestID, "kind", kind)
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func (s *Solver) serve(ctx context.Context, body quoteRequest, indicative bool) (response quoteResponse, err error) {
	ctx, end := tracer.Start(ctx, "iris.quote", observability.AttrOrderID.String(body.RequestID))
	defer func() {
		var refused *declined
		if errors.As(err, &refused) {
			observability.Decline(ctx, "declined", refused.Error())
			end(nil)
			return
		}
		end(err)
	}()
	r, err := parseRequest(body, s.chainID, indicative)
	if err != nil {
		return quoteResponse{}, &declined{venue: "solver", reason: "invalid_request", cause: err}
	}
	return s.quote(ctx, r, indicative)
}

func writeJSON(ctx context.Context, w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		observability.ErrorUnlessCanceled(ctx, err, "write iris response")
	}
}
