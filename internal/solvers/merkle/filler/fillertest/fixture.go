package fillertest

import (
	"context"
	"math/big"
	"os"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/connector/connectortest"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
)

// Settlement and RecipeExecutor are the fixture's protocol executor and its funding recipe.
var (
	Settlement     = common.HexToAddress("0x10")
	RecipeExecutor = common.HexToAddress("0x43")
)

// Workflow returns a connector workflow over a fake chain whose recipe pins Settlement as its
// caller and payment recipient. The chain accepts only a Settlement call carrying selector,
// sent by the signer, as the outer simulation. The oracle pays 500 for any input; the fixture
// discount of 2% leaves a connector payment of 490.
func Workflow(t *testing.T, selector [4]byte) (*connector.Workflow, *connectortest.Chain) {
	t.Helper()
	terms, err := connector.ParseDiscount(connectortest.DiscountFixture(), *connectortest.DiscountID(), connectortest.Venue, connectortest.TokenIn)
	if err != nil {
		t.Fatal(err)
	}
	fixture := connectortest.NewFixture(t, terms.Swap, terms.Signature, RecipeExecutor, Settlement, selector)
	fixture.Chain.AmountOut = big.NewInt(500)
	return &connector.Workflow{Chain: fixture.Chain, ChainID: big.NewInt(1), Source: fixture.Permissions, Filler: Settlement, Signer: fixture.Chain.Signer}, fixture.Chain
}

// The second route of WorkflowPair: another connector funded by another account.
var (
	SecondVenue          = common.HexToAddress("0x52")
	SecondRecipeExecutor = common.HexToAddress("0x53")
	SecondDiscountID     = common.HexToHash("0x54")
)

// WorkflowPair is Workflow with a second connector route. Both connectors pay 98% of the input
// they receive (one output unit per input unit, less the 2% fixture discount), so a fill can be
// planned across them with 18-decimal tokens; set each chain's Remaining to size the routes.
func WorkflowPair(t *testing.T, selector [4]byte) (*connector.Workflow, connectortest.Group, *Discounts) {
	t.Helper()
	first := connectortest.DiscountFixture()
	second := connectortest.DiscountFixture()
	second.DiscountID, second.Discount.Adapter = SecondDiscountID.Hex(), SecondVenue.Hex()
	routes := []struct {
		venue, account, executor common.Address
		id                       common.Hash
		resolved                 *discounts.Resolved
	}{
		{connectortest.Venue, common.HexToAddress("0x21"), RecipeExecutor, *connectortest.DiscountID(), first},
		{SecondVenue, common.HexToAddress("0x22"), SecondRecipeExecutor, SecondDiscountID, second},
	}
	chains := make([]*connectortest.Chain, 0, len(routes))
	permissions := make(connectortest.PermissionSet, 0, len(routes))
	provider := &Discounts{ResolvedByID: map[string]*discounts.Resolved{}}
	for _, route := range routes {
		terms, err := connector.ParseDiscount(route.resolved, route.id, route.venue, connectortest.TokenIn)
		if err != nil {
			t.Fatal(err)
		}
		fixture := connectortest.NewFixtureAt(t, route.venue, route.account, terms.Swap, terms.Signature, route.executor, Settlement, selector)
		fixture.Chain.AmountOutFor = func(amountIn *big.Int) *big.Int { return new(big.Int).Set(amountIn) }
		chains = append(chains, fixture.Chain)
		permissions = append(permissions, fixture.Permissions)
		provider.ResolvedByID[route.id.Hex()] = route.resolved
		provider.Listed = append(provider.Listed, discounts.ListItem{
			DiscountPrecision: "100000000", Adapter: route.venue.Hex(), TokenToRedeem: connectortest.TokenIn.Hex(),
			Collateral: connectortest.TokenOut.Hex(), DiscountID: route.id.Hex(), MaxAssets: "100000",
			Discount: "2000000", CollateralDecimals: 18, Deadline: 4102444800,
		})
	}
	group := connectortest.NewGroup(chains...)
	workflow := &connector.Workflow{Chain: group, ChainID: big.NewInt(1), Source: permissions, Filler: Settlement, Signer: chains[0].Signer}
	return workflow, group, provider
}

// Discounts is a scripted connector discount service advertising the fixture discount.
type Discounts struct {
	Listed   []discounts.ListItem
	ListErr  error
	Resolved *discounts.Resolved
	// ResolvedByID, when set, serves each discount its own signed terms.
	ResolvedByID map[string]*discounts.Resolved
	// ResolveErr fails the signed-terms request, as an unreachable discount service would.
	ResolveErr error
	Lists      int
	// Resolves counts signed-terms requests.
	Resolves int
}

// NewDiscounts advertises the fixture connector discount with capacity for 900 of the output.
func NewDiscounts() *Discounts {
	return &Discounts{
		Listed: []discounts.ListItem{{
			DiscountPrecision: "100000000", Adapter: connectortest.Venue.Hex(), TokenToRedeem: connectortest.TokenIn.Hex(),
			Collateral: connectortest.TokenOut.Hex(), DiscountID: connectortest.DiscountID().Hex(), MaxAssets: "900",
		}},
		Resolved: connectortest.DiscountFixture(),
	}
}

func (d *Discounts) ListDiscounts(context.Context) (*discounts.List, error) {
	d.Lists++
	if d.ListErr != nil {
		return nil, d.ListErr
	}
	return &discounts.List{Discounts: d.Listed}, nil
}

func (d *Discounts) Resolve(_ context.Context, id string) (*discounts.Resolved, error) {
	d.Resolves++
	if d.ResolveErr != nil {
		return nil, d.ResolveErr
	}
	if d.ResolvedByID != nil {
		return d.ResolvedByID[id], nil
	}
	return d.Resolved, nil
}

// ExampleConfig returns the config node of the single solver in an example config file, with
// its ${VAR} placeholders filled in: *_URL variables with a URL, everything else with an address.
func ExampleConfig(t *testing.T, path string) yaml.Node {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	expanded := os.Expand(string(raw), func(name string) string {
		if strings.HasSuffix(name, "_URL") {
			return "https://backend.example"
		}
		return "0x0000000000000000000000000000000000000010"
	})
	type exampleSolver struct {
		Config yaml.Node `yaml:"config"`
	}
	var document struct {
		Solvers []exampleSolver `yaml:"solvers"`
	}
	if err := yaml.Unmarshal([]byte(expanded), &document); err != nil || len(document.Solvers) != 1 {
		t.Fatal("example config has no single solver", err)
	}
	return document.Solvers[0].Config
}
