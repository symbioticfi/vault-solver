package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/api/bindings/rfq/executor"
	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	liquidplanning "github.com/symbioticfi/vault-solver/internal/liquidlane/planning"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

func mustBig(t *testing.T, raw string) *big.Int {
	t.Helper()
	value, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		t.Fatalf("invalid integer %q", raw)
	}
	return value
}

var (
	tIn  = common.HexToAddress("0x0000000000000000000000000000000000000001")
	tOut = common.HexToAddress("0x0000000000000000000000000000000000000002")
	vlt  = common.HexToAddress("0x0000000000000000000000000000000000000003")
)

type fakeQuoteCandidateReader struct {
	out           map[common.Address]*big.Int
	inputDecimals int
	queries       [][]liquidlane.Route
}

func (f *fakeQuoteCandidateReader) readQuoteCandidates(
	_ context.Context,
	inventory []liquidlane.Inventory,
	tokenIn, tokenOut common.Address,
	amount *big.Int,
	reservations liquidlane.CapacityReservations,
) ([]liquidlane.QuoteCandidate, error) {
	matching := make([]liquidlane.Inventory, 0, len(inventory))
	decimals := f.inputDecimals
	if decimals == 0 {
		decimals = 18
	}
	for _, item := range inventory {
		if item.TokenIn == tokenIn && item.TokenOut == tokenOut {
			item.TokenInDecimals = decimals
			matching = append(matching, item)
		}
	}
	matching = liquidplanning.AllocateInventoryCapacity(matching, reservations, 0)
	quotes := make([]liquidlane.FillQuote, 0, len(matching))
	routes := make([]liquidlane.Route, 0, len(matching))
	for _, item := range matching {
		routes = append(routes, item.Route)
		if amountOut := f.out[item.TokenOut]; amountOut != nil {
			quotes = append(quotes, liquidlane.FillQuote{Inventory: liquidlane.Inventory{Route: item.Route, MaxAssets: maxUint256()}, AmountIn: liquidlane.CloneBig(amount), MaxAmountOut: liquidlane.CloneBig(amountOut)})
		}
	}
	f.queries = append(f.queries, routes)
	return liquidplanning.NormalizeOracleInventory(amount, matching, quotes), nil
}

func newDefaultTestStrategy() Planner  { return newDefaultPlanner() }
func testCapacityBook() *capacity.Book { return new(capacity.Book) }
func testOrder(s *store) *orderRecord  { return s.orders["o1"] }
func maxUint256() *big.Int {
	return new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
}

func sampleOrder() executor.IReactorOrder {
	return executor.IReactorOrder{
		Request:          executor.IReactorRequest{TokenIn: tIn, AmountIn: big.NewInt(1e18), Outputs: []executor.IReactorOutput{{Token: tOut, Amount: big.NewInt(900000), Recipient: common.HexToAddress("0x99")}}, Deadline: big.NewInt(4102444800), Nonce: big.NewInt(1), Protocol: common.HexToAddress("0xaa")},
		SwapperSignature: []byte{1, 2}, Swapper: common.HexToAddress("0x99"), Filler: common.HexToAddress("0x10"),
		Outputs: []executor.IReactorOutput{{Token: tOut, Amount: big.NewInt(900000), Recipient: common.HexToAddress("0x99")}},
	}
}

type inputRecordingStrategy struct {
	quoteInput QuoteInput
	fillInput  FillInput
	quoteOut   QuoteOutput
	plan       *liquidlane.Plan
}

func (s *inputRecordingStrategy) DecideQuote(_ context.Context, input QuoteInput) (QuoteOutput, error) {
	s.quoteInput = input
	return s.quoteOut, nil
}

func (s *inputRecordingStrategy) BuildFillPlan(_ context.Context, input FillInput) (*liquidlane.Plan, error) {
	s.fillInput = input
	return s.plan, nil
}

func testPermissionedPolicy(t interface {
	Helper()
	Fatalf(format string, args ...any)
}, tokens ...common.Address) tokenpolicy.Policy {
	t.Helper()
	policy, err := tokenpolicy.New(tokenpolicy.Permissioned, tokens)
	if err != nil {
		t.Fatalf("token policy: %v", err)
	}
	return policy
}
