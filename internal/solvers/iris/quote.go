package iris

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/iris/blm"
)

const (
	// quoteWords is the EIP-712 encoding of Quote: the type hash and 19 fields, data hashed.
	quoteWords = 20
	// solverWord is the Quote word the sign leaf pins to the account.
	solverWord = 2
	// signBufferOffset is where the sign call's buffer contents start: selector, domain, two
	// head offsets and the buffer length.
	signBufferOffset = 4 + 4*32
)

var (
	domainTypeHash = crypto.Keccak256Hash([]byte("EIP712Domain(uint256 chainId,address verifyingContract)"))
	quoteTypeHash  = crypto.Keccak256Hash([]byte(
		"Quote(address borrower,address solver,address receiver,address blm,address collateralToken,address debtToken," +
			"uint256 collateral,uint256 debt,uint256 fixedRate,uint256 duration,uint256 overdueRate,uint256 overduePeriod," +
			"uint256 bond,uint256 bondLltv,uint256 venueBitmap,uint256 venueId,uint256 deadline,uint256 nonce,bytes data)",
	))
	// quoteInputs are the Quote words the offer recipe binds to inputs 0 to 8, in order:
	// borrower, receiver, collateral, debt, duration, fixedRate, venueBitmap, deadline, nonce.
	quoteInputs = [...]int{1, 3, 7, 8, 10, 9, 15, 17, 18}
)

const (
	debtInput, durationInput, fixedRateInput, deadlineInput = 3, 4, 5, 7
	// bondWord is the Quote word the recipe computes; the solver reproduces it with padBond.
	bondWord = 13
)

// Quote is the Iris Quote a solver signs and a borrower takes.
type Quote struct {
	Borrower, Solver, Receiver, Blm, CollateralToken, DebtToken common.Address

	Collateral, Debt, FixedRate, Duration, OverdueRate, OverduePeriod, Bond, BondLltv *big.Int
	VenueBitmap, VenueID, Deadline, Nonce                                             *big.Int

	Data []byte
}

func domainSeparator(chainID *big.Int, iris common.Address) common.Hash {
	return crypto.Keccak256Hash(domainTypeHash[:], common.BigToHash(chainID).Bytes(), common.LeftPadBytes(iris[:], 32))
}

// words returns the EIP-712 struct encoding, in the layout the account's sign buffer uses.
func (q Quote) words() [quoteWords]common.Hash {
	address := func(a common.Address) common.Hash { return common.BytesToHash(a[:]) }
	return [quoteWords]common.Hash{
		quoteTypeHash, address(q.Borrower), address(q.Solver), address(q.Receiver), address(q.Blm),
		address(q.CollateralToken), address(q.DebtToken), common.BigToHash(q.Collateral), common.BigToHash(q.Debt),
		common.BigToHash(q.FixedRate), common.BigToHash(q.Duration), common.BigToHash(q.OverdueRate),
		common.BigToHash(q.OverduePeriod), common.BigToHash(q.Bond), common.BigToHash(q.BondLltv),
		common.BigToHash(q.VenueBitmap), common.BigToHash(q.VenueID), common.BigToHash(q.Deadline),
		common.BigToHash(q.Nonce), crypto.Keccak256Hash(q.Data),
	}
}

// Digest is the EIP-712 hash Iris checks with the solver's isValidSignature.
func (q Quote) Digest(domain common.Hash) common.Hash {
	words := q.words()
	encoded := make([]byte, 0, quoteWords*32)
	for _, word := range words {
		encoded = append(encoded, word[:]...)
	}
	return crypto.Keccak256Hash([]byte{0x19, 0x01}, domain[:], crypto.Keccak256(encoded))
}

// inputs returns the recipe inputs: the keeper's Quote words as decimal strings.
func (q Quote) inputs() map[uint]string {
	words := q.words()
	inputs := make(map[uint]string, len(quoteInputs))
	for i, word := range quoteInputs {
		inputs[uint(i)] = words[word].Big().String()
	}
	return inputs
}

func (q Quote) binding() blm.Quote {
	return blm.Quote{
		Borrower: q.Borrower, Solver: q.Solver, Receiver: q.Receiver, Blm: q.Blm,
		CollateralToken: q.CollateralToken, DebtToken: q.DebtToken, Collateral: q.Collateral, Debt: q.Debt,
		FixedRate: q.FixedRate, Duration: q.Duration, OverdueRate: q.OverdueRate, OverduePeriod: q.OverduePeriod,
		Bond: q.Bond, BondLltv: q.BondLltv, VenueBitmap: q.VenueBitmap, VenueId: q.VenueID,
		Deadline: q.Deadline, Nonce: q.Nonce, Data: q.Data,
	}
}
