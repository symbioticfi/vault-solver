package iris

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func testQuote(m Market) Quote {
	return Quote{
		Borrower: common.HexToAddress("0xB0B"), Solver: m.Account, Receiver: common.HexToAddress("0xB0C"), Blm: m.Blm,
		CollateralToken: m.CollateralToken, DebtToken: m.DebtToken, Collateral: big.NewInt(1e8), Debt: big.NewInt(10_000e6),
		FixedRate: big.NewInt(6e16), Duration: big.NewInt(30 * 86400), OverdueRate: m.OverdueRate,
		OverduePeriod: m.OverduePeriod, Bond: big.NewInt(30e6), BondLltv: m.BondLltv, VenueBitmap: big.NewInt(2),
		VenueID: new(big.Int).SetUint64(m.VenueID), Deadline: big.NewInt(1_800_000_120), Nonce: big.NewInt(7), Data: m.Data,
	}
}

// The account computes the digest from the recipe's sign buffer with the inputs bound
// into it and the hash plan applied; the solver's digest must be the same hash.
func TestDigestMatchesTheAccountSignBuffer(t *testing.T) {
	f := loadFixture(t)
	prepared, tree := fixtureMarket(t, f)
	m, err := decodeMarket(prepared, tree, fixtureIris, big.NewInt(1))
	must(t, err)
	q := testQuote(m)

	buffer := append([]byte(nil), f.SignData[signBufferOffset:signBufferOffset+quoteWords*32+len(m.Data)]...)
	for i, w := range freeWords {
		value, parsed := new(big.Int).SetString(q.inputs()[uint(i)], 10)
		ok(t, parsed)
		copy(buffer[32*w:32*(w+1)], common.BigToHash(value).Bytes())
	}
	copy(buffer[32*dataHashWord:], crypto.Keccak256(buffer[quoteWords*32:]))
	structHash := crypto.Keccak256(buffer[:quoteWords*32])
	account := crypto.Keccak256Hash([]byte{0x19, 0x01}, m.Domain[:], structHash)

	equal(t, account, q.Digest(m.Domain))
}

func TestDigestMatchesAPITypes(t *testing.T) {
	q := testQuote(Market{
		Account: common.HexToAddress("0xACC"), Blm: common.HexToAddress("0x702"), CollateralToken: common.HexToAddress("0x703"),
		DebtToken: common.HexToAddress("0x704"), OverdueRate: big.NewInt(5e16), OverduePeriod: big.NewInt(604800),
		BondLltv: big.NewInt(9e17), VenueID: 1, Data: []byte{0xde, 0xad},
	})
	field := func(name, kind string) apitypes.Type { return apitypes.Type{Name: name, Type: kind} }
	dec := func(v *big.Int) *math.HexOrDecimal256 { return (*math.HexOrDecimal256)(v) }
	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {field("chainId", "uint256"), field("verifyingContract", "address")},
			"Quote": {
				field("borrower", "address"), field("solver", "address"), field("receiver", "address"), field("blm", "address"),
				field("collateralToken", "address"), field("debtToken", "address"), field("collateral", "uint256"),
				field("debt", "uint256"), field("fixedRate", "uint256"), field("duration", "uint256"),
				field("overdueRate", "uint256"), field("overduePeriod", "uint256"), field("bond", "uint256"),
				field("bondLltv", "uint256"), field("venueBitmap", "uint256"), field("venueId", "uint256"),
				field("deadline", "uint256"), field("nonce", "uint256"), field("data", "bytes"),
			},
		},
		PrimaryType: "Quote",
		Domain:      apitypes.TypedDataDomain{ChainId: dec(big.NewInt(1)), VerifyingContract: fixtureIris.Hex()},
		Message: apitypes.TypedDataMessage{
			"borrower": q.Borrower.Hex(), "solver": q.Solver.Hex(), "receiver": q.Receiver.Hex(), "blm": q.Blm.Hex(),
			"collateralToken": q.CollateralToken.Hex(), "debtToken": q.DebtToken.Hex(), "collateral": q.Collateral.String(),
			"debt": q.Debt.String(), "fixedRate": q.FixedRate.String(), "duration": q.Duration.String(),
			"overdueRate": q.OverdueRate.String(), "overduePeriod": q.OverduePeriod.String(), "bond": q.Bond.String(),
			"bondLltv": q.BondLltv.String(), "venueBitmap": q.VenueBitmap.String(), "venueId": q.VenueID.String(),
			"deadline": q.Deadline.String(), "nonce": q.Nonce.String(), "data": hexutil.Encode(q.Data),
		},
	}
	want, _, err := apitypes.TypedDataAndHash(typed)
	must(t, err)
	equal(t, common.BytesToHash(want), q.Digest(domainSeparator(big.NewInt(1), fixtureIris)))
}

func TestInputsFollowTheRecipeOrder(t *testing.T) {
	q := testQuote(Market{Account: common.HexToAddress("0xACC"), OverdueRate: new(big.Int), OverduePeriod: new(big.Int), BondLltv: new(big.Int)})
	inputs := q.inputs()
	equal(t, len(freeWords), len(inputs))
	want := []string{
		new(big.Int).SetBytes(q.Borrower.Bytes()).String(), new(big.Int).SetBytes(q.Receiver.Bytes()).String(),
		q.Collateral.String(), q.Debt.String(), q.FixedRate.String(), q.Duration.String(), q.Bond.String(),
		q.VenueBitmap.String(), q.Deadline.String(), q.Nonce.String(),
	}
	for i, value := range want {
		equal(t, value, inputs[uint(i)], "input %d", i)
	}
}
