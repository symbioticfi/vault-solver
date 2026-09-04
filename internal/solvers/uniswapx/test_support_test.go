package uniswapx

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/symbioticfi/vault-solver/internal/capacity"
)

func testCapacityBook() *capacity.Book { return new(capacity.Book) }

func testLifecycle(records map[common.Hash]orderLifecycle) orderLedger {
	if records == nil {
		records = make(map[common.Hash]orderLifecycle)
	}
	return orderLedger{records: records}
}

func testOrderLifecycle(solver *Solver, hash common.Hash) orderLifecycle {
	solver.ledger.mu.Lock()
	defer solver.ledger.mu.Unlock()
	return solver.ledger.records[hash]
}

func setTestExecution(solver *Solver, hash common.Hash, execution trackedOrder) {
	solver.ledger.mu.Lock()
	record := solver.ledger.records[hash]
	record.execution = execution
	if solver.ledger.records == nil {
		solver.ledger.records = make(map[common.Hash]orderLifecycle)
	}
	solver.ledger.records[hash] = record
	solver.ledger.mu.Unlock()
}

func testExclusiveOrderEntry(t interface {
	Helper()
	Fatal(...any)
}, exclusiveFiller common.Address) (orderEntry, *Config) {
	t.Helper()
	amount := big.NewInt(1)
	order := v2Order{Info: v2OrderInfo{Nonce: amount, Deadline: big.NewInt(1200)}, BaseInput: v2Input{StartAmount: amount, EndAmount: amount}, BaseOutputs: []v2Output{{StartAmount: amount, EndAmount: amount}}, CosignerData: v2CosignerData{DecayStartTime: big.NewInt(1000), DecayEndTime: big.NewInt(1100), ExclusiveFiller: exclusiveFiller, ExclusivityOverrideBps: new(big.Int), InputOverride: new(big.Int), OutputOverrides: []*big.Int{new(big.Int)}}}
	hash, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := v2OrderArguments.Pack(order)
	if err != nil {
		t.Fatal(err)
	}
	return orderEntry{Type: orderTypeDutchV2, EncodedOrder: hexutil.Encode(encoded), Signature: "0x", OrderHash: hash.Hex(), OrderStatus: orderStatusOpen, ChainID: 1, QuoteID: "quote-1"}, &Config{Executor: common.HexToAddress("0x2222222222222222222222222222222222222222")}
}
