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

func encodeOperationData(auth operationAuth, legs []selectedLeg, authSig []byte) ([]byte, error) {
	if len(legs) == 0 {
		return nil, errors.New("operationData: no legs")
	}
	if auth.BidAmount == nil || auth.BidAmount.Sign() <= 0 ||
		auth.MinBundleProfit == nil || auth.MinBundleProfit.Sign() <= 0 ||
		auth.Deadline == nil || auth.Deadline.Sign() <= 0 {
		return nil, errors.New("operationData: invalid auth")
	}
	if err := validateOperationLegs(legs); err != nil {
		return nil, err
	}
	enc, err := operationDataArgs.Pack(operationData{Auth: auth, Legs: legs, AuthSig: authSig})
	if err != nil {
		return nil, errors.Errorf("encode operationData: %w", err)
	}
	return enc, nil
}

func callbackAuthDigest(chainID *big.Int, callback, executor common.Address, auth operationAuth, legs []selectedLeg) (common.Hash, error) {
	if err := validateOperationLegs(legs); err != nil {
		return common.Hash{}, err
	}
	legsHash, err := encodedLegsHash(legs)
	if err != nil {
		return common.Hash{}, err
	}
	enc, err := authDigestArgs.Pack(
		authDomain, chainID, callback, executor, auth.AuctionKey, auth.BidAmount, auth.MinBundleProfit,
		auth.Deadline, legsHash,
	)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth digest: %w", err)
	}
	return crypto.Keccak256Hash(enc), nil
}

func validateOperationLegs(legs []selectedLeg) error {
	for i, leg := range legs {
		if leg.MarketId == (common.Hash{}) {
			return errors.Errorf("operationData: invalid leg %d marketId", i)
		}
		if leg.Borrower == (common.Address{}) {
			return errors.Errorf("operationData: invalid leg %d borrower", i)
		}
		if leg.MaxSeizeAssets == nil || leg.MaxSeizeAssets.Sign() <= 0 {
			return errors.Errorf("operationData: invalid leg %d maxSeizeAssets", i)
		}
		if leg.MinProfit == nil || leg.MinProfit.Sign() <= 0 {
			return errors.Errorf("operationData: invalid leg %d minProfit", i)
		}
	}
	return nil
}

func encodedLegsHash(legs []selectedLeg) (common.Hash, error) {
	enc, err := callbackLegArrayArgs.Pack(legs)
	if err != nil {
		return common.Hash{}, errors.Errorf("encode callback auth legs: %w", err)
	}
	return crypto.Keccak256Hash(enc), nil
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
