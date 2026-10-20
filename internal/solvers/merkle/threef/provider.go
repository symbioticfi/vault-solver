package threef

import (
	"context"
	"encoding/json"
	"math/big"
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

// API is read-only during preparation. Offer mutations belong to the host loop.
type API interface {
	Identity(ctx context.Context, account common.Address) error
	Offers(context.Context) ([]threef.OfferDto, error)
}

type Provider struct {
	API   API
	Chain provider.Reader
	Now   func() time.Time
}

type mint struct {
	Request         common.Address `json:"request"`
	Policy          common.Address `json:"policy"`
	Asset           common.Address `json:"asset"`
	UnderlyingAsset common.Address `json:"underlyingAsset"`
	Principal       string         `json:"principal"`
	Yield           string         `json:"yield"`
	Deadline        time.Time      `json:"deadline"`
}

func (p *Provider) Prepare(context.Context, provider.Snapshot) (provider.Output, error) {
	return provider.Output{}, errors.New("3f requires authenticated recipe preparation")
}
func (p *Provider) RecipeRequirements(snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree) (keeper.RecipeRequirements, error) {
	market, err := Inspect(snapshot, recipe, tree)
	if err != nil {
		return keeper.RecipeRequirements{}, err
	}
	return keeper.RecipeRequirements{Inputs: []uint{market.RequestInput, market.PrincipalInput, market.YieldInput}}, nil
}

func (p *Provider) PrepareRecipe(ctx context.Context, snapshot provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree, _ map[uint]*big.Int) (provider.Output, error) {
	market, err := Inspect(snapshot, recipe, tree)
	if err != nil {
		return provider.Output{}, err
	}
	if err = p.API.Identity(ctx, snapshot.Account); err != nil {
		return provider.Output{}, err
	}
	offers, err := p.API.Offers(ctx)
	if err != nil {
		return provider.Output{}, err
	}
	deadlines := map[common.Address]time.Time{}
	for _, offer := range offers {
		if !strings.EqualFold(offer.Maker, snapshot.Account.Hex()) || offer.Status != "AWAITING_MINT" {
			continue
		}
		if offer.ExecutionMode != "PUSH" || !common.IsHexAddress(offer.RequestId) || common.HexToAddress(offer.RequestId) == (common.Address{}) {
			return provider.Output{}, errors.New("3f invalid pending PUSH request")
		}
		deadline := offer.MintDeadline.Get()
		if deadline == nil {
			return provider.Output{}, errors.New("3f pending mint has no lease deadline")
		}
		parsed, e := time.Parse(time.RFC3339Nano, *deadline)
		if e != nil {
			return provider.Output{}, errors.New("3f invalid mint lease deadline")
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
		return provider.Output{}, provider.ErrNotReady
	}
	observed, err := Observe(ctx, p.Chain, snapshot, recipe, tree)
	if err != nil {
		return provider.Output{}, err
	}
	for _, address := range addresses {
		deadline := deadlines[address]
		if !p.time().Add(30*time.Second).Before(deadline) || snapshot.Timestamp >= uint64(deadline.Unix()) {
			continue
		}
		raw, e := p.Chain.CallContract(ctx, ethereum.CallMsg{To: &address, Data: requestABI.PackMintAuthorization(snapshot.Account)}, snapshot.BlockNumber)
		if e != nil {
			return provider.Output{}, errors.New("3f read mintAuthorization failed")
		}
		auth, e := requestABI.UnpackMintAuthorization(raw)
		if e != nil || len(raw) != 64 || auth.PtAmount == nil || auth.YtAmount == nil || auth.PtAmount.Sign() < 0 || auth.PtAmount.BitLen() > 128 || auth.YtAmount.Sign() < 0 || auth.YtAmount.BitLen() > 128 {
			return provider.Output{}, errors.New("3f invalid mintAuthorization")
		}
		if auth.PtAmount.Sign() == 0 {
			continue
		}
		if auth.PtAmount.Cmp(observed.Budget) > 0 || auth.PtAmount.Cmp(observed.MaxPrincipal) > 0 || new(big.Int).Mul(auth.YtAmount, big.NewInt(100_000_000)).Cmp(new(big.Int).Mul(auth.PtAmount, observed.MinimumYield)) < 0 {
			continue
		}
		if e = Eligible(ctx, p.Chain, snapshot, market, address, observed.MinimumYield); e != nil {
			if errors.Is(e, ErrIneligible) {
				continue
			}
			return provider.Output{}, e
		}
		record := mint{Request: address, Policy: market.Policy, Asset: market.Asset, UnderlyingAsset: market.UnderlyingAsset, Principal: auth.PtAmount.String(), Yield: auth.YtAmount.String(), Deadline: deadline}
		data, e := json.Marshal(record)
		if e != nil {
			return provider.Output{}, e
		}
		return provider.Output{Inputs: map[uint]string{market.RequestInput: new(big.Int).SetBytes(address.Bytes()).String(), market.PrincipalInput: auth.PtAmount.String(), market.YieldInput: auth.YtAmount.String()}, Data: data}, nil
	}
	return provider.Output{}, provider.ErrNotReady
}

// ValidateLease runs again immediately before unsigned admission. Signed recovery
// retains ownership and never rebuilds or resubmits an external offer.
func (p *Provider) ValidateLease(ctx context.Context, snapshot provider.Snapshot, output provider.Output) error {
	record, err := decodeMint(output)
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

func (p *Provider) Advance(_ context.Context, snapshot provider.Snapshot, output provider.Output, _ json.RawMessage) (provider.Progress, error) {
	record, err := decodeMint(output)
	if err != nil {
		return provider.Progress{}, err
	}
	if err = snapshot.Execution.Validate(); err != nil {
		return provider.Progress{}, err
	}
	principal, ok := new(big.Int).SetString(record.Principal, 10)
	if !ok || principal.Sign() <= 0 {
		return provider.Progress{}, errors.New("3f invalid persisted principal")
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
		if decoded.Asset == record.Asset && decoded.UnderlyingAsset == record.UnderlyingAsset && decoded.Assets.Cmp(principal) == 0 {
			count++
		}
	}
	if count != 1 {
		return provider.Progress{}, errors.New("3f confirmed mint lacks one exact policy debit")
	}
	return provider.Progress{Done: true, Outcome: "minted", Reference: snapshot.Execution.TxHash.Hex()}, nil
}

func decodeMint(output provider.Output) (mint, error) {
	var record mint
	if err := json.Unmarshal(output.Data, &record); err != nil {
		return record, errors.New("3f invalid persisted mint")
	}
	if record.Request == (common.Address{}) || record.Policy == (common.Address{}) || record.Asset == (common.Address{}) || record.UnderlyingAsset == (common.Address{}) || record.Deadline.IsZero() {
		return record, errors.New("3f incomplete persisted mint")
	}
	return record, nil
}
func (p *Provider) time() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}
