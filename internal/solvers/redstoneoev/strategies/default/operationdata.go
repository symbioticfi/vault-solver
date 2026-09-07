package defaultstrategy

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

type operationAuth struct {
	AuctionKey      common.Hash
	BidAmount       *big.Int
	MinBundleProfit *big.Int
	Deadline        *big.Int
}

type operationData struct {
	Auth    operationAuth
	Legs    []selectedLeg
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
		{Type: mustType("uint256")},
		{Type: mustType("bytes32")},
	}
	authDomain = crypto.Keccak256Hash([]byte("SYMBIOTIC_OEV_AUTH_V1"))
)

func encodeOperationData(auth operationAuth, legs []selectedLeg, signature []byte) ([]byte, error) {
	if len(legs) == 0 {
		return nil, errors.New("operationData: no legs")
	}
	for _, amount := range []*big.Int{auth.BidAmount, auth.MinBundleProfit, auth.Deadline} {
		if amount == nil || amount.Sign() <= 0 || amount.BitLen() > 256 {
			return nil, errors.New("operationData: invalid auth")
		}
	}
	if err := validateOperationLegs(legs); err != nil {
		return nil, err
	}
	payload := operationData{Auth: auth, Legs: legs, AuthSig: signature}
	encoded, err := operationDataArgs.Pack(payload)
	if err != nil {
		return nil, errors.Errorf("encode operationData: %w", err)
	}
	return encoded, nil
}

func callbackAuthDigest(chainID *big.Int, callback, executor common.Address, auth operationAuth, legs []selectedLeg) (common.Hash, error) {
	if err := validateOperationLegs(legs); err != nil {
		return common.Hash{}, err
	}
	encodedLegs, err := callbackLegArrayArgs.Pack(legs)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth legs: %w", err)
	}
	fields := []any{authDomain, chainID, callback, executor, auth.AuctionKey, auth.BidAmount,
		auth.MinBundleProfit, auth.Deadline, crypto.Keccak256Hash(encodedLegs)}
	encoded, err := authDigestArgs.Pack(fields...)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth digest: %w", err)
	}
	return crypto.Keccak256Hash(encoded), nil
}

func validateOperationLegs(legs []selectedLeg) error {
	for index, leg := range legs {
		invalid := ""
		switch {
		case leg.MarketId == (common.Hash{}):
			invalid = "marketId"
		case leg.Borrower == (common.Address{}):
			invalid = "borrower"
		case leg.MaxSeizeAssets == nil || leg.MaxSeizeAssets.Sign() <= 0 || leg.MaxSeizeAssets.BitLen() > 256:
			invalid = "maxSeizeAssets"
		case leg.MinProfit == nil || leg.MinProfit.Sign() <= 0 || leg.MinProfit.BitLen() > 256:
			invalid = "minProfit"
		}
		if invalid != "" {
			return errors.Errorf("operationData: invalid leg %d %s", index, invalid)
		}
	}
	return nil
}

func auctionKeyHash(id string) common.Hash {
	return crypto.Keccak256Hash([]byte("id:" + id))
}

func callbackAuthDeadline(now time.Time, ttl time.Duration) *big.Int {
	return big.NewInt(now.Add(ttl).Unix())
}

func mustOperationDataType() abi.Type {
	t, err := abi.NewType("tuple", "", []abi.ArgumentMarshaling{
		{Name: "auth", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "auctionKey", Type: "bytes32"},
			{Name: "bidAmount", Type: "uint256"},
			{Name: "minBundleProfit", Type: "uint256"},
			{Name: "deadline", Type: "uint256"},
		}},
		{Name: "legs", Type: "tuple[]", Components: callbackLegComponents()},
		{Name: "authSig", Type: "bytes"},
	})
	if err != nil {
		panic("redstoneoev/defaultstrategy: build OperationData type: " + err.Error())
	}
	return t
}

func mustCallbackLegArrayType() abi.Type {
	t, err := abi.NewType("tuple[]", "", callbackLegComponents())
	if err != nil {
		panic("redstoneoev/defaultstrategy: build LiquidationLeg[] type: " + err.Error())
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

func mustType(t string) abi.Type {
	typ, err := abi.NewType(t, "", nil)
	if err != nil {
		panic("redstoneoev/defaultstrategy: abi type " + t + ": " + err.Error())
	}
	return typ
}
