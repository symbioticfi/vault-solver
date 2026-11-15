package threef

import (
	"context"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/3f/request"
	"github.com/symbioticfi/vault-solver/api/threef"
)

var requestABI = request.NewIRequest()

// mintAPI is read-only during preparation. Offer mutations belong to the host loop.
type mintAPI interface {
	Identity(ctx context.Context, account common.Address) error
	Offers(context.Context) ([]threef.OfferDto, error)
	Auctions(context.Context) ([]threef.AuctionDto, error)
}

type mintProvider struct {
	API    mintAPI
	Chain  provider.Reader
	Policy common.Address
	Now    func() time.Time
}

type mint struct {
	Request         common.Address
	Policy          common.Address
	Asset           common.Address
	UnderlyingAsset common.Address
	Principal       *big.Int
	Deadline        time.Time
}

// The configured offer policy must also be the policy charged by every mint group.
func (p *mintProvider) markets(snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree) ([]mintMarket, error) {
	markets, err := inspect(snapshot, recipe, tree)
	if err != nil {
		return nil, err
	}
	for _, market := range markets {
		if market.Policy != p.Policy {
			return nil, errors.New("3f recipe policy differs from configured policy")
		}
	}
	return markets, nil
}

func (p *mintProvider) RecipeRequirements(snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree) (keeper.RecipeRequirements, error) {
	markets, err := p.markets(snapshot, recipe, tree)
	if err != nil {
		return keeper.RecipeRequirements{}, err
	}
	var inputs []uint
	for _, market := range markets {
		for _, index := range market.inputs() {
			if !slices.Contains(inputs, index) {
				inputs = append(inputs, index)
			}
		}
	}
	return keeper.RecipeRequirements{Inputs: inputs}, nil
}

func (p *mintProvider) PrepareRecipe(ctx context.Context, snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree, _ map[uint]*big.Int) ([]provider.Output, error) {
	markets, err := p.markets(snapshot, recipe, tree)
	if err != nil {
		return nil, err
	}
	if err = p.API.Identity(ctx, snapshot.Account); err != nil {
		return nil, err
	}
	offers, err := p.API.Offers(ctx)
	if err != nil {
		return nil, err
	}
	deadlines := map[common.Address]time.Time{}
	for _, offer := range offers {
		if !strings.EqualFold(offer.Maker, snapshot.Account.Hex()) || offer.Status != "AWAITING_MINT" {
			continue
		}
		if offer.ExecutionMode != "PUSH" || !common.IsHexAddress(offer.RequestId) || common.HexToAddress(offer.RequestId) == (common.Address{}) {
			return nil, errors.New("3f invalid pending PUSH request")
		}
		deadline := offer.MintDeadline.Get()
		if deadline == nil {
			return nil, errors.New("3f pending mint has no lease deadline")
		}
		parsed, e := time.Parse(time.RFC3339Nano, *deadline)
		if e != nil {
			return nil, errors.New("3f invalid mint lease deadline")
		}
		address := common.HexToAddress(offer.RequestId)
		if old, ok := deadlines[address]; !ok || parsed.Before(old) {
			deadlines[address] = parsed
		}
	}
	addresses := make([]common.Address, 0, len(deadlines))
	for address := range deadlines {
		addresses = append(addresses, address)
	}
	sort.Slice(addresses, func(i, j int) bool {
		if !deadlines[addresses[i]].Equal(deadlines[addresses[j]]) {
			return deadlines[addresses[i]].Before(deadlines[addresses[j]])
		}
		return addresses[i].Cmp(addresses[j]) < 0
	})
	if len(addresses) == 0 {
		return nil, provider.ErrNotReady
	}
	inputs := map[uint]string{}
	for _, market := range markets {
		for _, index := range market.inputs() {
			inputs[index] = "0"
		}
	}
	markets, err = p.resolveMarkets(ctx, snapshot, markets, offers, deadlines)
	if err != nil {
		return nil, err
	}
	observations := make([]observation, 0, len(markets))
	for _, market := range markets {
		observed, e := observe(ctx, p.Chain, snapshot, recipe, market)
		if e != nil {
			return nil, e
		}
		observations = append(observations, observed)
	}
	for _, address := range addresses {
		deadline := deadlines[address]
		if !p.time().Add(30*time.Second).Before(deadline) || snapshot.Timestamp >= uint64(deadline.Unix()) {
			continue
		}
		raw, e := p.Chain.CallContract(ctx, ethereum.CallMsg{To: &address, Data: requestABI.PackMintAuthorization(snapshot.Account)}, snapshot.BlockNumber)
		if e != nil {
			return nil, errors.New("3f read mintAuthorization failed")
		}
		auth, e := requestABI.UnpackMintAuthorization(raw)
		if e != nil {
			return nil, errors.New("3f invalid mintAuthorization")
		}
		if auth.PtAmount.Sign() == 0 {
			continue
		}
		for _, observed := range observations {
			market := observed.Market
			if auth.PtAmount.Cmp(observed.Budget) > 0 || auth.PtAmount.Cmp(observed.MaxPrincipal) > 0 || auth.YtAmount.Cmp(minimumReturn(auth.PtAmount, observed.MinimumYield)) < 0 {
				continue
			}
			if e = eligible(ctx, p.Chain, snapshot, market, address, observed.MinimumYield); e != nil {
				if errors.Is(e, errIneligible) {
					continue
				}
				return nil, e
			}
			record := mint{Request: address, Policy: market.Policy, Asset: market.Asset, UnderlyingAsset: market.UnderlyingAsset, Principal: new(big.Int).Set(auth.PtAmount), Deadline: deadline}
			inputs[market.RequestInput] = new(big.Int).SetBytes(address.Bytes()).String()
			results := map[uint]string{}
			for i, value := range []recipes.Value{market.Principal, market.Yield} {
				amount := []*big.Int{auth.PtAmount, auth.YtAmount}[i].String()
				index := uint(value.Value.Uint64())
				if value.Source == recipes.Input {
					inputs[index] = amount
				} else {
					// The SDK checks these against the full execution simulation.
					results[index] = amount
				}
			}
			return []provider.Output{{Inputs: inputs, Results: results, Task: &record}}, nil
		}
	}
	return nil, provider.ErrNotReady
}

// A dynamic mint leaf commits the funding token, not a fixed collateral pair.
// Resolve candidates from pending auctions; eligible still attests the exact
// request and pair through the policy before any input is supplied.
func (p *mintProvider) resolveMarkets(ctx context.Context, snapshot provider.Snapshot, markets []mintMarket, offers []threef.OfferDto, pending map[common.Address]time.Time) ([]mintMarket, error) {
	if !slices.ContainsFunc(markets, func(m mintMarket) bool { return m.UnderlyingAsset == (common.Address{}) }) {
		return markets, nil
	}
	auctions, err := p.API.Auctions(ctx)
	if err != nil {
		return nil, err
	}
	auctions = slices.DeleteFunc(auctions, func(a threef.AuctionDto) bool {
		_, ok := pending[common.HexToAddress(a.RequestId)]
		return !ok
	})
	pairs, err := discoverMarkets(ctx, p.Chain, snapshot, p.Policy, auctions, offers)
	if err != nil {
		return nil, err
	}
	var resolved []mintMarket
	for _, market := range markets {
		if market.UnderlyingAsset != (common.Address{}) {
			resolved = append(resolved, market)
			continue
		}
		for _, pair := range pairs {
			if pair.Asset == market.Asset {
				candidate := market
				candidate.UnderlyingAsset = pair.UnderlyingAsset
				resolved = append(resolved, candidate)
			}
		}
	}
	return resolved, nil
}

// ValidateLease closes preparation after recipe simulation. The keeper retains
// the admitted task in RAM without rebuilding or resubmitting an external offer.
func (p *mintProvider) ValidateLease(ctx context.Context, snapshot provider.Snapshot, output provider.Output) error {
	record, err := mintFromOutput(output)
	if err != nil {
		return err
	}
	if !p.time().Add(30 * time.Second).Before(record.Deadline) {
		return provider.ErrNotReady
	}
	if err = p.API.Identity(ctx, snapshot.Account); err != nil {
		return err
	}
	offers, err := p.API.Offers(ctx)
	if err != nil {
		return err
	}
	found := false
	earliest := record.Deadline
	for _, offer := range offers {
		if !strings.EqualFold(offer.Maker, snapshot.Account.Hex()) || !strings.EqualFold(offer.RequestId, record.Request.Hex()) || offer.Status != "AWAITING_MINT" {
			continue
		}
		if offer.ExecutionMode != "PUSH" || offer.MintDeadline.Get() == nil {
			return provider.ErrNotReady
		}
		deadline, e := time.Parse(time.RFC3339Nano, *offer.MintDeadline.Get())
		if e != nil {
			return errors.New("3f invalid mint lease deadline")
		}
		if deadline.Before(earliest) {
			earliest = deadline
		}
		found = true
	}
	if !found || !p.time().Add(30*time.Second).Before(earliest) {
		return provider.ErrNotReady
	}
	return nil
}

func (record *mint) Advance(_ context.Context, snapshot provider.Snapshot) (provider.Progress, error) {
	if err := record.validate(); err != nil {
		return provider.Progress{}, err
	}
	if err := snapshot.Execution.Validate(); err != nil {
		return provider.Progress{}, err
	}
	count := 0
	event := policySchema.Events["Consume"]
	for _, log := range snapshot.Execution.Logs {
		if log.Address != record.Policy || len(log.Topics) == 0 || log.Topics[0] != event.ID {
			continue
		}
		decoded, e := policyABI.UnpackConsumeEvent(&log)
		if e != nil {
			return provider.Progress{}, errors.New("3f invalid policy consume receipt")
		}
		if decoded.Asset == record.Asset && decoded.UnderlyingAsset == record.UnderlyingAsset && decoded.Assets.Cmp(record.Principal) == 0 {
			count++
		}
	}
	if count != 1 {
		return provider.Progress{}, errors.New("3f confirmed mint lacks one exact policy debit")
	}
	return provider.Progress{Done: true, Outcome: "minted", Reference: snapshot.Execution.TxHash.Hex()}, nil
}

func mintFromOutput(output provider.Output) (*mint, error) {
	record, ok := output.Task.(*mint)
	if !ok || record == nil {
		return nil, errors.New("3f mint task is required")
	}
	if err := record.validate(); err != nil {
		return nil, err
	}
	return record, nil
}

func (record *mint) validate() error {
	if record == nil || record.Request == (common.Address{}) || record.Policy == (common.Address{}) || record.Asset == (common.Address{}) || record.UnderlyingAsset == (common.Address{}) || record.Deadline.IsZero() {
		return errors.New("3f incomplete mint task")
	}
	if record.Principal == nil || record.Principal.Sign() <= 0 || record.Principal.BitLen() > 256 {
		return errors.New("3f invalid mint principal")
	}
	return nil
}
func (p *mintProvider) time() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
