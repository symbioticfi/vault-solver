package threef

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strconv"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"
	accountbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/merkleaccount"
	executorbinding "github.com/symbioticfi/merkle-executor/sdk/bindings/recipeexecutor"
	"github.com/symbioticfi/merkle-executor/sdk/keeper"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/threef"
)

type offerPassData struct {
	pin     provider.Snapshot
	recipe  recipes.Recipe
	tree    *recipes.Tree
	markets []mintMarket
	market  offerMarket
}

func offerPassFixture(t *testing.T) (*offerWorkflow, *fakeClient, offerPassData) {
	t.Helper()
	pin, recipe, tree := fixture(t)
	pin.RecipeHash = common.HexToHash("0xcafe")
	markets, err := inspect(pin, recipe, tree)
	if err != nil {
		t.Fatal(err)
	}
	b, client, m := testBot(t)
	m.Account, m.Executor, m.RecipeHash, m.Asset, m.UnderlyingAsset = pin.Account, pin.Executor, pin.RecipeHash, markets[0].Asset, markets[0].UnderlyingAsset
	m.Policy = markets[0].Policy
	m.Budget, m.AvailableCash, m.MinimumYield1e8 = big.NewInt(45), big.NewInt(123), big.NewInt(2_000_000)
	client.auctions = nil
	return &offerWorkflow{bot: b}, client, offerPassData{pin, recipe, tree, markets, m}
}

func eligibilityReads(t *testing.T, pin provider.Snapshot, recipe recipes.Recipe, tree *recipes.Tree, run func(ethereum.CallMsg) error) readFunc {
	t.Helper()
	reads := marketReads(t, pin, recipe, tree, func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
		t.Fatal("unexpected read")
		return nil, nil
	})
	return func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if msg.To != nil && *msg.To == pin.Account && bytes.Equal(msg.Data[:4], capabilitySchema.Methods["execute"].ID) {
			if err := run(msg); err != nil {
				return nil, err
			}
		}
		return reads(ctx, msg, block)
	}
}

func TestOfferPassRecoveryRechecksNativeMembership(t *testing.T) {
	for _, allowed := range []bool{true, false} {
		t.Run(strconv.FormatBool(allowed), func(t *testing.T) {
			w, client, data := offerPassFixture(t)
			request := common.HexToAddress("0x1000000000000000000000000000000000000001")
			auction := testAuction(data.market, 1, request)
			auction.AdditionalProperties[exactMaxRateKey] = "1000"
			client.createErr = errors.New("HTTP 500")
			if err := w.bot.Tick(t.Context(), data.market, []threef.AuctionDto{auction}); err == nil {
				t.Fatal("missing initial POST failure")
			}
			client.createErr = nil
			checks := 0
			chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(ethereum.CallMsg) error {
				checks++
				if !allowed {
					return policyRevertError{}
				}
				return nil
			})
			err := w.evaluate(t.Context(), chain, data.pin, data.markets, []threef.AuctionDto{auction}, nil, nil)
			if (err == nil) != allowed || checks != 1 {
				t.Fatalf("recovery membership checks=%d err=%v", checks, err)
			}
			want := 1
			if allowed {
				want = 2
			}
			if len(client.created) != want {
				t.Fatalf("got %d attempts, want %d", len(client.created), want)
			}
		})
	}
}

func TestOfferPassSkipsHistoricalAndForeignFundingAuctions(t *testing.T) {
	w, c, data := offerPassFixture(t)
	pin, recipe, tree, markets, m := data.pin, data.recipe, data.tree, data.markets, data.market
	historical := testAuction(m, 1, common.HexToAddress("0x101"))
	historical.Status = "SUCCEEDED"
	foreign := m
	foreign.Asset = common.HexToAddress("0x999")
	auctions := []threef.AuctionDto{historical, testAuction(foreign, 2, common.HexToAddress("0x102"))}
	calls := 0
	chain := eligibilityReads(t, pin, recipe, tree, func(ethereum.CallMsg) error { calls++; return policyRevertError{} })
	if err := w.evaluate(t.Context(), chain, pin, markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 0 || len(c.created) != 0 {
		t.Fatalf("unrelated catalog consumed eligibility work: calls=%d created=%d", calls, len(c.created))
	}
}

func TestOfferPassMemoizesNegativeMembershipPerPin(t *testing.T) {
	w, c, data := offerPassFixture(t)
	pin, recipe, tree, markets, m := data.pin, data.recipe, data.tree, data.markets, data.market
	auctions := []threef.AuctionDto{}
	for i := 1; i <= 50; i++ {
		auctions = append(auctions, testAuction(m, float32(i), common.HexToAddress("0x101")))
	}
	calls := 0
	chain := eligibilityReads(t, pin, recipe, tree, func(ethereum.CallMsg) error { calls++; return policyRevertError{} })
	if err := w.evaluate(t.Context(), chain, pin, markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("one rejected request simulated %d times at one pin", calls)
	}
	pin.BlockNumber = big.NewInt(11)
	// A later pin must observe membership again, including prior negative results.
	chain = eligibilityReads(t, pin, recipe, tree, func(ethereum.CallMsg) error { calls++; return policyRevertError{} })
	if err := w.evaluate(t.Context(), chain, pin, markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("eligibility cache escaped its pin: %d calls", calls)
	}
}

func TestOfferPassMaintainsCommitmentBeforeOptionalAuctionFailure(t *testing.T) {
	w, c, data := offerPassFixture(t)
	pin, recipe, tree, markets, m := data.pin, data.recipe, data.tree, data.markets, data.market
	request := common.HexToAddress("0x1000000000000000000000000000000000000001")
	live := testAuction(m, 1, request)
	live.Status = "SOLVED"
	optional := common.HexToAddress("0x102")
	auctions := []threef.AuctionDto{live, testAuction(m, 2, optional)}
	c.offers = []threef.OfferDto{testOffer(m, 1, "SUBMITTED", "46")}
	optionalCalls := 0
	chain := eligibilityReads(t, pin, recipe, tree, func(msg ethereum.CallMsg) error {
		if bytes.Contains(msg.Data, common.LeftPadBytes(optional.Bytes(), 32)) {
			optionalCalls++
			return errors.New("optional RPC unavailable")
		}
		return nil
	})
	if err := w.evaluate(t.Context(), chain, pin, markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if len(c.cancelled) != 1 || optionalCalls != 0 {
		t.Fatalf("maintenance starved: cancelled=%v optional calls=%d", c.cancelled, optionalCalls)
	}
}

func TestOfferPassChecksAllUnansweredAuctions(t *testing.T) {
	for _, count := range []int{1, 257} {
		t.Run(fmt.Sprintf("auctions%d", count), func(t *testing.T) {
			w, c, data := offerPassFixture(t)
			auctions := make([]threef.AuctionDto, 0, count)
			for i := 1; i <= count; i++ {
				auctions = append(auctions, testAuction(data.market, float32(i), common.BigToAddress(big.NewInt(int64(100+i)))))
			}
			calls := 0
			chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(ethereum.CallMsg) error {
				calls++
				return policyRevertError{}
			})
			if err := w.evaluate(t.Context(), chain, data.pin, data.markets, auctions, c.offers, nil); err != nil {
				t.Fatal(err)
			}
			if calls != count {
				t.Fatalf("checked %d of %d unanswered auctions in one pass", calls, count)
			}
		})
	}
}

func TestOfferPassChecksAllCandidatesBeforePosting(t *testing.T) {
	w, c, data := offerPassFixture(t)
	auctions := make([]threef.AuctionDto, 0, 3)
	for i := 1; i <= 3; i++ {
		auction := testAuction(data.market, float32(i), common.BigToAddress(big.NewInt(int64(100+i))))
		auction.AdditionalProperties[exactMaxRateKey] = "10000"
		auctions = append(auctions, auction)
	}
	calls := 0
	chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(ethereum.CallMsg) error { calls++; return nil })
	c.beforeCreate = func(threef.CreateOfferDto) {
		if calls != 3 {
			t.Fatalf("posted before checking all auctions: %d eligibility checks", calls)
		}
	}
	if err := w.evaluate(t.Context(), chain, data.pin, data.markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if len(c.created) != 1 || c.created[0].AuctionId != 1 {
		t.Fatalf("unexpected first offer: %+v", c.created)
	}
}

func TestOfferPassSkipsAuctionsAlreadyAnsweredByMaker(t *testing.T) {
	w, c, data := offerPassFixture(t)
	answered, unanswered := common.HexToAddress("0x101"), common.HexToAddress("0x102")
	auctions := []threef.AuctionDto{testAuction(data.market, 1, answered), testAuction(data.market, 2, unanswered)}
	own := testOffer(data.market, 1, "CANCELLED", "1")
	own.RequestId = answered.Hex()
	foreign := testOffer(data.market, 2, "SUBMITTED", "1")
	foreign.Maker, foreign.RequestId = common.HexToAddress("0x999").Hex(), unanswered.Hex()
	c.offers = []threef.OfferDto{own, foreign}
	calls := 0
	chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(msg ethereum.CallMsg) error {
		calls++
		if !bytes.Contains(msg.Data, common.LeftPadBytes(unanswered.Bytes(), 32)) {
			t.Fatal("rechecked an auction already answered by this maker")
		}
		return policyRevertError{}
	})
	if err := w.evaluate(t.Context(), chain, data.pin, data.markets, auctions, c.offers, nil); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("unanswered auction affected by another maker's offer: %d checks", calls)
	}
}

func TestOfferPassRechecksSnapshotAfterOptionalEligibility(t *testing.T) {
	w, c, data := offerPassFixture(t)
	auction := testAuction(data.market, 1, common.HexToAddress("0x101"))
	auction.AdditionalProperties[exactMaxRateKey] = "10000"
	chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(ethereum.CallMsg) error { return nil })
	checks := 0
	changed := errors.New("snapshot changed")
	err := w.evaluate(t.Context(), chain, data.pin, data.markets, []threef.AuctionDto{auction}, c.offers, func(context.Context) error {
		checks++
		if checks > 1 {
			return changed
		}
		return nil
	})
	if !errors.Is(err, changed) || checks != 2 || len(c.created) != 0 || len(c.cancelled) != 0 {
		t.Fatalf("snapshot fence lost: checks=%d err=%v", checks, err)
	}
}

func TestOfferPassMemoizesPositiveMembershipPerPin(t *testing.T) {
	w, c, data := offerPassFixture(t)
	auctions := []threef.AuctionDto{}
	for i := 1; i <= 50; i++ {
		auctions = append(auctions, testAuction(data.market, float32(i), common.HexToAddress("0x101")))
	}
	calls := 0
	chain := eligibilityReads(t, data.pin, data.recipe, data.tree, func(ethereum.CallMsg) error { calls++; return nil })
	validations := 0
	if err := w.evaluate(t.Context(), chain, data.pin, data.markets, auctions, c.offers, func(context.Context) error { validations++; return nil }); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || validations != 2 || len(c.created) != 0 {
		t.Fatalf("positive membership/stage repeated or floor bypassed: calls=%d validations=%d created=%d", calls, validations, len(c.created))
	}
}

type offerTestChain struct {
	readFunc

	header *types.Header
	logs   []types.Log
}

func (c offerTestChain) HeaderByHash(context.Context, common.Hash) (*types.Header, error) {
	return c.header, nil
}
func (c offerTestChain) HeaderByNumber(context.Context, *big.Int) (*types.Header, error) {
	return c.header, nil
}
func (c offerTestChain) FilterLogs(context.Context, ethereum.FilterQuery) ([]types.Log, error) {
	return c.logs, nil
}

func TestOfferTickDoesNotNeedPermissionService(t *testing.T) {
	w, client, data := offerPassFixture(t)
	pin, recipe, market := data.pin, data.recipe, data.markets[0]
	pin.Caller = common.HexToAddress("0xcafe")
	header := &types.Header{Number: pin.BlockNumber, Time: pin.Timestamp, Difficulty: new(big.Int)}
	executor := executorbinding.NewRecipeExecutor().GetABI()
	account := accountbinding.NewMerkleAccount().GetABI()
	hash, err := recipes.HashRecipe(recipe)
	if err != nil {
		t.Fatal(err)
	}
	event := executor.Events["SetRecipe"]
	encoded, err := event.Inputs.NonIndexed().Pack(recipe.Queries, recipe.Steps)
	if err != nil {
		t.Fatal(err)
	}
	log := types.Log{Address: pin.Executor, BlockNumber: pin.BlockNumber.Uint64(), BlockHash: header.Hash(), Topics: []common.Hash{event.ID, hash}, Data: encoded}
	manager, collateral := common.HexToAddress("0x91"), common.HexToAddress("0x92")
	auction := testAuction(data.market, 1, common.HexToAddress("0x101"))
	auction.Asset.Set(&threef.ResolvedAssetDto{PositionManager: manager.Hex()})
	auction.AdditionalProperties[exactMaxRateKey] = "10000"
	client.auctions = []threef.AuctionDto{auction}
	base := eligibilityReads(t, pin, recipe, data.tree, func(ethereum.CallMsg) error { return nil })
	chain := offerTestChain{header: header, logs: []types.Log{log}}
	chain.readFunc = func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		switch *msg.To {
		case pin.Executor:
			method, err := executor.MethodById(msg.Data)
			if err != nil {
				return nil, err
			}
			switch method.Name {
			case "account":
				return method.Outputs.Pack(pin.Account)
			case "caller":
				return method.Outputs.Pack(pin.Caller)
			case "recipeHash":
				return method.Outputs.Pack(hash)
			case "recipeBlock":
				return method.Outputs.Pack(pin.BlockNumber)
			}
		case pin.Account:
			if method, err := account.MethodById(msg.Data); err == nil {
				switch method.Name {
				case "root":
					return method.Outputs.Pack(pin.Root)
				case "hasRole":
					return method.Outputs.Pack(true)
				}
			}
		case manager:
			return positionManagerABI.GetABI().Methods["assets"].Outputs.Pack(collateral, common.Address{})
		case collateral:
			return underlyingAssetABI.GetABI().Methods["underlying"].Outputs.Pack(market.UnderlyingAsset)
		}
		return base(ctx, msg, block)
	}
	// Deliberately no PermissionSource: an offer pass must not fetch a Merkle bundle.
	w.planner = &keeper.Planner{Chain: chain, ChainID: pin.ChainID, Caller: pin.Caller}
	w.strategy.Config.Executor, w.strategy.Config.Account = pin.Executor.Hex(), pin.Account.Hex()
	w.policy, w.api = market.Policy, client
	if err := w.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(client.created) != 1 || client.created[0].Amount != "45" {
		t.Fatalf("policy capacity was not offered: %+v", client.created)
	}
}
