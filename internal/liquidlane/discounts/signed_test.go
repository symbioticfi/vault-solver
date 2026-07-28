package discounts

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

type fakeProvider struct {
	resolved  *Resolved
	requested string
}

func (f *fakeProvider) ListDiscounts(context.Context) (*List, error) {
	return &List{}, nil
}

func (f *fakeProvider) Resolve(_ context.Context, discountID string) (*Resolved, error) {
	f.requested = discountID
	return f.resolved, nil
}

func TestValidateSignedChecksSelectionDeadlinesAndOutput(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	baseInventory := testPhysicalInventory()
	id := common.HexToHash(testOfferID)
	signed := &Signed{
		DiscountID: id,
		Adapter:    baseInventory.Adapter,
		Terms: SignedTerms{
			TokenToRedeem: baseInventory.TokenIn,
			Discount:      big.NewInt(100_000),
			Deadline:      big.NewInt(now.Add(time.Minute).Unix()),
		},
		ProtocolDeadline: big.NewInt(now.Add(2 * time.Minute).Unix()),
	}
	base := liquidlane.FillQuote{
		Inventory: baseInventory,
		AmountIn:  big.NewInt(1_000), GrossAmountOut: big.NewInt(1_000),
		MinDiscount: big.NewInt(100_000),
	}
	selection := Selection{
		DiscountID: id, Adapter: baseInventory.Adapter, TokenIn: baseInventory.TokenIn,
		MinAmountOut: big.NewInt(900),
	}
	amountOut, err := ValidateSigned(signed, selection, base, now)
	if err != nil || amountOut.Cmp(big.NewInt(900)) != 0 {
		t.Fatalf("amountOut=%v err=%v", amountOut, err)
	}

	selection.MinAmountOut = big.NewInt(901)
	if _, err := ValidateSigned(signed, selection, base, now); err == nil {
		t.Fatal("expected minimum output rejection")
	}
	selection.MinAmountOut = big.NewInt(900)
	if _, err := ValidateSigned(signed, selection, base, now.Add(time.Minute)); err == nil {
		t.Fatal("expected deadline rejection")
	}
}

func TestFindFillQuoteRequiresExactRouteAndAmount(t *testing.T) {
	base := testPhysicalInventory()
	quote := liquidlane.FillQuote{Inventory: base, AmountIn: big.NewInt(10)}
	if _, ok := FindFillQuote(
		[]liquidlane.FillQuote{quote}, base.Adapter, base.TokenIn, base.TokenOut, big.NewInt(10),
	); !ok {
		t.Fatal("expected matching quote")
	}
	if _, ok := FindFillQuote(
		[]liquidlane.FillQuote{quote}, base.Adapter, base.TokenIn, base.TokenOut, big.NewInt(11),
	); ok {
		t.Fatal("unexpected amount mismatch")
	}
}

func TestResolveSelectedBindsFreshTermsToExactPhysicalQuote(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	base := testPhysicalInventory()
	id := common.HexToHash(testOfferID)
	provider := &fakeProvider{resolved: &Resolved{
		DiscountID: testOfferID,
		Discount: Terms{
			Adapter: base.Adapter.Hex(), TokenToRedeem: base.TokenIn.Hex(), Discount: "100000",
			Signer:   common.HexToAddress("0x5555555555555555555555555555555555555555").Hex(),
			Protocol: common.HexToAddress("0x6666666666666666666666666666666666666666").Hex(),
			Nonce:    "0x1", Deadline: now.Add(time.Minute).Unix(),
		},
		SignerSignature: "0x1234", ProtocolDeadline: now.Add(time.Minute).Unix(),
		ProtocolSignature: "0x5678",
	}}
	physical := []liquidlane.FillQuote{{
		Inventory: base, AmountIn: big.NewInt(1_000), GrossAmountOut: big.NewInt(1_000),
		MaxAmountOut: big.NewInt(900), MinDiscount: big.NewInt(100_000),
	}}
	selection := Selection{
		DiscountID: id, Adapter: base.Adapter, TokenIn: base.TokenIn, TokenOut: base.TokenOut,
		AmountIn: big.NewInt(1_000), MinAmountOut: big.NewInt(900),
	}

	signed, err := ResolveSelected(context.Background(), provider, selection, physical, now)
	if err != nil || signed == nil || provider.requested != testOfferID {
		t.Fatalf("signed=%+v requested=%q err=%v", signed, provider.requested, err)
	}
	selection.AmountIn = big.NewInt(999)
	if _, err := ResolveSelected(context.Background(), provider, selection, physical, now); err == nil {
		t.Fatal("expected exact amount binding failure")
	}
}
