package redstoneoev

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

// LiquidationLeg is one solver-selected liquidation cap. The callback receives only the signed fields
// encoded by callbackLeg; SwapAmountOut is a solver-only estimate for selection and gas prediction.
type LiquidationLeg struct {
	MarketId       common.Hash
	Borrower       common.Address
	MaxSeizeAssets *big.Int
	MinProfit      *big.Int
	SwapAmountOut  *big.Int
}

type operationAuth struct {
	AuctionKey      common.Hash
	BidAmount       *big.Int
	MinBundleProfit *big.Int
}

type callbackLeg struct {
	MarketId       common.Hash
	Borrower       common.Address
	MaxSeizeAssets *big.Int
	MinProfit      *big.Int
}

type operationData struct {
	Auth    operationAuth
	Legs    []callbackLeg
	AuthSig []byte
}

var (
	operationDataArgs    = abi.Arguments{{Type: mustOperationDataType()}}
	callbackLegArrayArgs = abi.Arguments{{Type: mustCallbackLegArrayType()}}
	authDigestArgs       = abi.Arguments{
		{Type: mustType("bytes32")},
		{Type: mustType("uint256")},
		{Type: mustType("address")},
		{Type: mustType("address")},
		{Type: mustType("bytes32")},
		{Type: mustType("uint256")},
		{Type: mustType("uint256")},
		{Type: mustType("bytes32")},
	}
	authDomain = crypto.Keccak256Hash([]byte("SYMBIOTIC_OEV_AUTH_V2"))
)

// EncodeOperationData ABI-encodes the callback payload committed by the RedStone EXECUTOR_V6 signature.
func EncodeOperationData(auth operationAuth, legs []LiquidationLeg, authSig []byte) ([]byte, error) {
	if len(legs) == 0 {
		return nil, errors.New("operationData: no legs")
	}
	if auth.BidAmount == nil || auth.MinBundleProfit == nil || auth.MinBundleProfit.Sign() <= 0 {
		return nil, errors.New("operationData: invalid auth")
	}
	for i, leg := range legs {
		if leg.MinProfit == nil || leg.MinProfit.Sign() < 0 {
			return nil, errors.Errorf("operationData: invalid leg %d minProfit", i)
		}
	}
	op := operationData{Auth: auth, Legs: encodeLegs(legs), AuthSig: authSig}
	enc, err := operationDataArgs.Pack(op)
	if err != nil {
		return nil, errors.Errorf("encode operationData: %w", err)
	}
	return enc, nil
}

func CallbackAuthDigest(chainID *big.Int, callback, executor common.Address, auth operationAuth, legs []LiquidationLeg) (common.Hash, error) {
	legsHash, err := encodedLegsHash(legs)
	if err != nil {
		return common.Hash{}, err
	}
	enc, err := authDigestArgs.Pack(authDomain, chainID, callback, executor, auth.AuctionKey, auth.BidAmount, auth.MinBundleProfit, legsHash)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth digest: %w", err)
	}
	return crypto.Keccak256Hash(enc), nil
}

func encodedLegsHash(legs []LiquidationLeg) (common.Hash, error) {
	enc, err := callbackLegArrayArgs.Pack(encodeLegs(legs))
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth legs: %w", err)
	}
	return crypto.Keccak256Hash(enc), nil
}

func encodeLegs(legs []LiquidationLeg) []callbackLeg {
	out := make([]callbackLeg, len(legs))
	for i, leg := range legs {
		out[i] = callbackLeg{
			MarketId:       leg.MarketId,
			Borrower:       leg.Borrower,
			MaxSeizeAssets: leg.MaxSeizeAssets,
			MinProfit:      leg.MinProfit,
		}
	}
	return out
}

func auctionKeyHash(a AuctionMessage) common.Hash {
	return crypto.Keccak256Hash([]byte(a.dedupKey()))
}

func legsWithProfitFloors(legs []LiquidationLeg, gas gasPrediction, gasPrice, rate *big.Int) []LiquidationLeg {
	out := make([]LiquidationLeg, len(legs))
	copy(out, legs)
	for i := range out {
		route := gasRouteUnknown
		if i < len(gas.Routes) {
			route = gas.Routes[i]
		}
		units := gasUnitsForRoute(route)
		out[i].MinProfit = nativeToLoan(gasCostNative(units, gasPrice), rate)
	}
	return out
}

func mustOperationDataType() abi.Type {
	t, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "auth", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "auctionKey", Type: "bytes32"},
			{Name: "bidAmount", Type: "uint256"},
			{Name: "minBundleProfit", Type: "uint256"},
		}},
		{Name: "legs", Type: "tuple[]", Components: callbackLegComponents()},
		{Name: "authSig", Type: "bytes"},
	})
	if err != nil {
		panic("redstoneoev: build OperationData type: " + err.Error())
	}
	return t
}

func mustCallbackLegArrayType() abi.Type {
	t, err := abi.NewType("tuple[]", "", callbackLegComponents())
	if err != nil {
		panic("redstoneoev: build LiquidationLeg[] type: " + err.Error())
	}
	return t
}

func callbackLegComponents() []abi.ArgumentMarshaling {
	return []abi.ArgumentMarshaling{
		{Name: "marketId", Type: "bytes32"},
		{Name: "borrower", Type: "address"},
		{Name: "maxSeizeAssets", Type: "uint256"},
		{Name: "minProfit", Type: "uint256"},
	}
}
