package defaultstrategy

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/execution"
)

func TestEncodeOperationDataRoundTrip(t *testing.T) {
	auth := operationAuth{
		AuctionKey:      common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111"),
		BidAmount:       mustBig("500000000000000"),
		MinBundleProfit: mustBig("2200000"),
		Deadline:        mustBig("1781243700"),
	}
	legs := []selectedLeg{{
		MarketId:       common.HexToHash("0x6209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5"),
		Borrower:       common.HexToAddress("0x629d764eC8563AFA701709B52c1a215e865632dE"),
		MaxSeizeAssets: mustBig("500000000000000000"),
		MinProfit:      mustBig("625000"),
	}}
	authSig := bytes.Repeat([]byte{0x42}, 65)

	got, err := encodeOperationData(auth, legs, authSig)
	if err != nil {
		t.Fatal(err)
	}
	// Independently encoded with cast abi-encode for the Solidity OperationData tuple.
	want := "0x000000000000000000000000000000000000000000000000000000000000002011111111111111111111111111111111111111111111111111111111111111110000000000000000000000000000000000000000000000000001c6bf5263400000000000000000000000000000000000000000000000000000000000002191c0000000000000000000000000000000000000000000000000000000006a2b9f3400000000000000000000000000000000000000000000000000000000000000c000000000000000000000000000000000000000000000000000000000000001e0000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000206209dbd022c20923c071d7183d7a9729a75596136540d474a27d08ef31f440a5000000000000000000000000629d764ec8563afa701709b52c1a215e865632de00000000000000000000000000000000000000000000000006f05b59d3b200000000000000000000000000000000000000000000000000000000000000098968000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000041424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424242424200000000000000000000000000000000000000000000000000000000000000"
	if hexutil.Encode(got) != want {
		t.Fatalf("operationData ABI mismatch:\n got %s\nwant %s", hexutil.Encode(got), want)
	}
	back, err := decodeOperationData(got)
	if err != nil {
		t.Fatalf("decode operationData: %v", err)
	}
	if back.Auth.AuctionKey != auth.AuctionKey ||
		back.Auth.BidAmount.Cmp(auth.BidAmount) != 0 ||
		back.Auth.MinBundleProfit.Cmp(auth.MinBundleProfit) != 0 ||
		back.Auth.Deadline.Cmp(auth.Deadline) != 0 {
		t.Fatalf("auth round-trip mismatch: %+v", back.Auth)
	}
	if len(back.Legs) != 1 {
		t.Fatalf("legs len = %d, want 1", len(back.Legs))
	}
	if leg := back.Legs[0]; leg.MarketId != legs[0].MarketId ||
		leg.Borrower != legs[0].Borrower ||
		leg.MaxSeizeAssets.Cmp(legs[0].MaxSeizeAssets) != 0 ||
		leg.MinProfit.Cmp(legs[0].MinProfit) != 0 {
		t.Fatalf("leg round-trip mismatch: %+v", leg)
	}
	if !bytes.Equal(back.AuthSig, authSig) {
		t.Fatalf("authSig mismatch")
	}
}

func TestEncodeOperationDataRejectsMissingAuth(t *testing.T) {
	leg := selectedLeg{Borrower: common.Address{19: 1}, MaxSeizeAssets: big.NewInt(1), MinProfit: big.NewInt(1)}
	for name, auth := range map[string]operationAuth{
		"no bid":               {MinBundleProfit: big.NewInt(1), Deadline: big.NewInt(1)},
		"no min bundle profit": {BidAmount: big.NewInt(1), Deadline: big.NewInt(1)},
		"no deadline":          {BidAmount: big.NewInt(1), MinBundleProfit: big.NewInt(1)},
		"zero min bundle profit": {
			BidAmount:       big.NewInt(1),
			MinBundleProfit: big.NewInt(0),
			Deadline:        big.NewInt(1),
		},
		"zero deadline": {
			BidAmount:       big.NewInt(1),
			MinBundleProfit: big.NewInt(1),
			Deadline:        big.NewInt(0),
		},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := encodeOperationData(auth, []selectedLeg{leg}, nil); err == nil {
				t.Fatal("expected invalid auth error")
			}
		})
	}
	if _, err := encodeOperationData(operationAuth{
		BidAmount: big.NewInt(1), MinBundleProfit: big.NewInt(1), Deadline: big.NewInt(1),
	}, nil, nil); err == nil {
		t.Fatal("expected error for empty legs")
	}
}

func TestEncodeOperationDataRejectsInvalidLegs(t *testing.T) {
	auth := operationAuth{BidAmount: big.NewInt(1), MinBundleProfit: big.NewInt(1), Deadline: big.NewInt(1)}
	valid := selectedLeg{
		MarketId:       common.Hash{31: 1},
		Borrower:       common.Address{19: 1},
		MaxSeizeAssets: big.NewInt(1),
		MinProfit:      big.NewInt(1),
	}
	for name, mutate := range map[string]func(*selectedLeg){
		"nil maxSeizeAssets":  func(l *selectedLeg) { l.MaxSeizeAssets = nil },
		"zero maxSeizeAssets": func(l *selectedLeg) { l.MaxSeizeAssets = big.NewInt(0) },
		"nil minProfit":       func(l *selectedLeg) { l.MinProfit = nil },
		"zero minProfit":      func(l *selectedLeg) { l.MinProfit = big.NewInt(0) },
		"negative minProfit":  func(l *selectedLeg) { l.MinProfit = big.NewInt(-1) },
	} {
		t.Run(name, func(t *testing.T) {
			leg := valid
			mutate(&leg)
			if _, err := encodeOperationData(auth, []selectedLeg{leg}, nil); err == nil {
				t.Fatal("expected invalid leg error")
			}
		})
	}
	if _, err := encodeOperationData(auth, []selectedLeg{valid}, nil); err != nil {
		t.Fatalf("valid leg must encode: %v", err)
	}
}

func TestCallbackAuthDigestBindsLegs(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	auth := operationAuth{
		AuctionKey:      common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"),
		BidAmount:       big.NewInt(100),
		MinBundleProfit: big.NewInt(200),
		Deadline:        big.NewInt(300),
	}
	legs := []selectedLeg{{
		MarketId:       common.Hash{31: 1},
		Borrower:       common.Address{19: 2},
		MaxSeizeAssets: big.NewInt(3),
		MinProfit:      big.NewInt(4),
	}}
	digest, err := callbackAuthDigest(big.NewInt(11155111), common.Address{19: 3}, common.Address{19: 4}, auth, legs)
	if err != nil {
		t.Fatal(err)
	}
	sig, err := crypto.Sign(digest.Bytes(), key)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := crypto.SigToPub(digest.Bytes(), sig)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := crypto.PubkeyToAddress(*pub), crypto.PubkeyToAddress(key.PublicKey); got != want {
		t.Fatalf("recovered %s, want %s", got, want)
	}

	changed := legs
	changed[0].MinProfit.Add(changed[0].MinProfit, big.NewInt(1))
	changedDigest, err := callbackAuthDigest(big.NewInt(11155111), common.Address{19: 3}, common.Address{19: 4}, auth, changed)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == digest {
		t.Fatal("digest must change when leg minProfit changes")
	}
	changedAuth := auth
	changedAuth.Deadline = big.NewInt(301)
	changedDigest, err = callbackAuthDigest(big.NewInt(11155111), common.Address{19: 3}, common.Address{19: 4}, changedAuth, legs)
	if err != nil {
		t.Fatal(err)
	}
	if changedDigest == digest {
		t.Fatal("digest must change when auth deadline changes")
	}
}

func TestGenericExitDataRoundTripAndAuthorization(t *testing.T) {
	auth := operationAuth{AuctionKey: common.Hash{31: 1}, BidAmount: big.NewInt(1), MinBundleProfit: big.NewInt(1), Deadline: big.NewInt(123)}
	legs := []selectedLeg{{MarketId: common.Hash{31: 2}, Borrower: common.Address{19: 3}, MaxSeizeAssets: big.NewInt(100), MinProfit: big.NewInt(5), CollateralRecipient: common.Address{19: 4}, Calls: []execution.Call{{Target: common.Address{19: 5}, Value: big.NewInt(0), Data: []byte{1, 2, 3, 4}}}}}
	data, err := encodeOperationData(auth, legs, []byte{6})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeOperationData(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Legs[0].CollateralRecipient != legs[0].CollateralRecipient || len(decoded.Legs[0].Calls) != 1 || !bytes.Equal(decoded.Legs[0].Calls[0].Data, legs[0].Calls[0].Data) {
		t.Fatalf("exit not preserved: %+v", decoded.Legs)
	}
	before, err := callbackAuthDigest(big.NewInt(1), common.Address{19: 6}, common.Address{19: 7}, auth, legs)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(){func() { legs[0].CollateralRecipient[19]++ }, func() { legs[0].Calls[0].Target[19]++ }, func() { legs[0].Calls[0].Value.SetInt64(1) }, func() { legs[0].Calls[0].Data[0]++ }} {
		mutate()
		after, err := callbackAuthDigest(big.NewInt(1), common.Address{19: 6}, common.Address{19: 7}, auth, legs)
		if err != nil {
			t.Fatal(err)
		}
		if before == after {
			t.Fatal("exit mutation did not change authorization digest")
		}
		before = after
	}
}
