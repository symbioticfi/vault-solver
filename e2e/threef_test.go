//go:build e2e

package e2e

import (
	"math"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	threefadapter "github.com/symbioticfi/vault-solver/api/bindings/3f/adapter"
	threefrequest "github.com/symbioticfi/vault-solver/api/bindings/3f/request"
)

type threeFOfferWire struct {
	ID             int64          `json:"id"`
	AuctionID      int64          `json:"auctionId"`
	RequestID      common.Address `json:"requestId"`
	Maker          common.Address `json:"maker"`
	Amount         string         `json:"amount"`
	ExpectedReturn string         `json:"expectedReturn"`
	Nonce          string         `json:"nonce"`
	Expiration     string         `json:"expiration"`
	UseCallback    bool           `json:"useCallback"`
	Signature      string         `json:"signature"`
}

func testThreeF(t *testing.T, testEnv *testEnvironment) {
	t.Helper()
	if testEnv.variant != "protocol" {
		t.Fatalf("3f variant = %q, want protocol", testEnv.variant)
	}
	manifest := testEnv.manifest.ThreeF
	if manifest.Adapter == (common.Address{}) || manifest.Request == (common.Address{}) || manifest.Asset == (common.Address{}) {
		t.Fatal("3f deployment manifest is incomplete")
	}

	var health struct {
		OK bool `json:"ok"`
	}
	if status := testEnv.getJSON(t, testEnv.fixtureURL+"/health", &health); status != http.StatusOK || !health.OK {
		t.Fatalf("3f health status = %d, body = %+v", status, health)
	}

	var offer threeFOfferWire
	eventually(t, "signed 3f offer", 90*time.Second, 2*time.Second, func() error {
		var state struct {
			Offers []threeFOfferWire `json:"offers"`
		}
		if status := testEnv.getJSON(t, testEnv.fixtureURL+"/state", &state); status != http.StatusOK {
			return errors.Errorf("state status %d", status)
		}
		for _, candidate := range state.Offers {
			if candidate.RequestID == manifest.Request && candidate.Signature != "" {
				offer = candidate
				return nil
			}
		}
		return errors.Errorf("no signed offer among %d offers", len(state.Offers))
	})

	offerAmount := parseBig(t, offer.Amount)
	expectedReturn := parseBig(t, offer.ExpectedReturn)
	if offer.Maker != manifest.Adapter || offerAmount.Sign() <= 0 || expectedReturn.Sign() <= 0 {
		t.Fatalf("invalid 3f offer: %+v", offer)
	}
	adapter := threefadapter.NewThreeFAdapter()
	minimumYieldPPM, err := adapter.UnpackMinYieldPerRequest(
		testEnv.call(t, manifest.Adapter, adapter.PackMinYieldPerRequest()),
	)
	if err != nil {
		t.Fatalf("decode 3f minimum yield: %v", err)
	}
	minimumReturn := minYieldReturn(offerAmount, minimumYieldPPM)
	partialSafeReturn := partialSafeMinYieldReturn(offerAmount, minimumYieldPPM)
	if expectedReturn.Cmp(partialSafeReturn) != 0 {
		t.Fatalf("3f expected return = %s, want partial-safe %s (minimum %s)", expectedReturn, partialSafeReturn, minimumReturn)
	}
	maxRatePPM := big.NewInt(int64(math.Round(manifest.Auction.MaxRateBPS * 100)))
	maximumReturn := mulDivDown(offerAmount, maxRatePPM, big.NewInt(ppmScale))
	if expectedReturn.Cmp(maximumReturn) > 0 {
		t.Fatalf("3f expected return %s exceeds maximum %s", expectedReturn, maximumReturn)
	}

	consumedPrincipal := new(big.Int).Add(new(big.Int).Div(offerAmount, big.NewInt(2)), big.NewInt(1))
	verifyThreeFPartialYieldSafety(t, offerAmount, expectedReturn, minimumYieldPPM, consumedPrincipal)
	consumedYield := proratedYield(expectedReturn, consumedPrincipal, offerAmount)
	minimumConsumedYield := requiredYield(consumedPrincipal, minimumYieldPPM)
	if consumedPrincipal.Sign() <= 0 || consumedPrincipal.Cmp(offerAmount) >= 0 || consumedYield.Cmp(minimumConsumedYield) < 0 {
		t.Fatalf("unsafe 3f partial consume: principal=%s yield=%s minimum=%s", consumedPrincipal, consumedYield, minimumConsumedYield)
	}

	request := threefrequest.NewIRequest()
	signature, err := hexutil.Decode(offer.Signature)
	if err != nil {
		t.Fatalf("decode 3f offer signature: %v", err)
	}
	offerTuple := threefrequest.Offer{
		Maker:          offer.Maker,
		Amount:         offerAmount,
		ExpectedReturn: expectedReturn,
		Nonce:          parseBig(t, offer.Nonce),
		Expiration:     parseBig(t, offer.Expiration),
		UseCallback:    offer.UseCallback,
	}
	consumeReceipt := testEnv.send(
		t,
		anvilDeployerKey,
		manifest.Request,
		request.PackConsume(offerTuple, signature, consumedPrincipal),
	)

	testEnv.send(t, anvilDeployerKey, manifest.Asset, packFixtureMint(t, manifest.Request, consumedYield))

	var increased any
	if err := testEnv.client.Client().Call(&increased, "evm_increaseTime", hexutil.Uint64(2)); err != nil {
		t.Fatalf("increase Anvil time: %v", err)
	}
	var mined any
	if err := testEnv.client.Client().Call(&mined, "evm_mine"); err != nil {
		t.Fatalf("mine Anvil block: %v", err)
	}

	repayment := new(big.Int).Add(consumedPrincipal, consumedYield)
	requestBalance := testEnv.balanceOf(t, manifest.Asset, manifest.Request)
	if requestBalance.Cmp(repayment) < 0 {
		t.Fatalf("3f request balance %s is below repayment %s", requestBalance, repayment)
	}
	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	repaidReceipt := testEnv.send(
		t,
		anvilDeployerKey,
		manifest.Request,
		request.PackSetRepaid(repayment, maxUint256),
	)

	eventually(t, "3f adapter finalization", 90*time.Second, 2*time.Second, func() error {
		length, unpackErr := adapter.UnpackRequestsLength(
			testEnv.call(t, manifest.Adapter, adapter.PackRequestsLength()),
		)
		if unpackErr != nil {
			return errors.Errorf("decode requestsLength: %w", unpackErr)
		}
		if length.Sign() != 0 {
			return errors.Errorf("requestsLength is %s", length)
		}
		return nil
	})

	t.Logf(
		"3f flow offer=%d consume=%s repaid=%s principal=%s yield=%s",
		offer.ID,
		consumeReceipt.TxHash,
		repaidReceipt.TxHash,
		consumedPrincipal,
		consumedYield,
	)
}

func verifyThreeFPartialYieldSafety(
	t *testing.T,
	principal, expectedReturn, minimumYieldPPM, selectedPrincipal *big.Int,
) {
	t.Helper()
	margin := mulDivUp(principal, big.NewInt(1), big.NewInt(ppmScale))
	if margin.Cmp(big.NewInt(2)) < 0 {
		margin.SetInt64(2)
	}
	guaranteeThreshold := mulDivUp(principal, big.NewInt(1), margin)
	samples := []struct {
		name      string
		principal *big.Int
	}{
		{name: "guarantee-threshold", principal: guaranteeThreshold},
		{name: "threshold-plus-one", principal: new(big.Int).Add(guaranteeThreshold, big.NewInt(1))},
		{name: "rounding-boundary", principal: selectedPrincipal},
		{name: "almost-full", principal: new(big.Int).Sub(principal, big.NewInt(1))},
	}
	for _, sample := range samples {
		consumedYield := proratedYield(expectedReturn, sample.principal, principal)
		minimumYield := requiredYield(sample.principal, minimumYieldPPM)
		if consumedYield.Cmp(minimumYield) < 0 {
			t.Fatalf("3f %s partial yield = %s, want at least %s for principal %s", sample.name, consumedYield, minimumYield, sample.principal)
		}
	}

	floorExactYield := proratedYield(minYieldReturn(principal, minimumYieldPPM), selectedPrincipal, principal)
	selectedMinimum := requiredYield(selectedPrincipal, minimumYieldPPM)
	if floorExactYield.Cmp(selectedMinimum) >= 0 {
		t.Fatalf("3f rounding fixture does not distinguish floor-exact yield %s from required %s", floorExactYield, selectedMinimum)
	}
}

func packFixtureMint(t *testing.T, recipient common.Address, amount *big.Int) []byte {
	t.Helper()
	addressType, err := abi.NewType("address", "", nil)
	if err != nil {
		t.Fatalf("construct mint address ABI: %v", err)
	}
	uintType, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatalf("construct mint amount ABI: %v", err)
	}
	arguments, err := (abi.Arguments{{Type: addressType}, {Type: uintType}}).Pack(recipient, amount)
	if err != nil {
		t.Fatalf("pack fixture mint: %v", err)
	}
	selector := crypto.Keccak256([]byte("mint(address,uint256)"))[:4]
	return append(selector, arguments...)
}
