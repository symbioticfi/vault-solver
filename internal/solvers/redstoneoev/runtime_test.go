package redstoneoev

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	liquidlanegas "github.com/symbioticfi/vault-solver/internal/liquidlane/gas"
	strategytypes "github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

func TestCoherentStateSourceRequiresStableHeadIdentity(t *testing.T) {
	start := stateTestHeader(100, 1)
	same := ethtypes.CopyHeader(start)
	nextBlock := ethtypes.CopyHeader(start)
	nextBlock.Number = big.NewInt(101)
	sameHeightReorg := ethtypes.CopyHeader(start)
	sameHeightReorg.Root = common.BytesToHash([]byte{2})
	if start.Hash() == sameHeightReorg.Hash() {
		t.Fatal("same-height reorg fixture did not change the header hash")
	}

	for _, test := range []struct {
		name         string
		end          *ethtypes.Header
		gasPrices    *liquidlanegas.PriceSnapshot
		wantBoundary bool
	}{
		{name: "stable head without gas", end: same},
		{name: "matching gas batch", end: same, gasPrices: &liquidlanegas.PriceSnapshot{BlockTime: time.Unix(int64(start.Time), 0)}},
		{name: "older fallback gas batch", end: same, gasPrices: &liquidlanegas.PriceSnapshot{BlockTime: time.Unix(int64(start.Time)-12, 0)}, wantBoundary: true},
		{name: "newer gas batch", end: same, gasPrices: &liquidlanegas.PriceSnapshot{BlockTime: time.Unix(int64(start.Time)+12, 0)}, wantBoundary: true},
		{name: "missing gas batch time", end: same, gasPrices: &liquidlanegas.PriceSnapshot{}, wantBoundary: true},
		{name: "next block", end: nextBlock, wantBoundary: true},
		{name: "same-height reorg", end: sameHeightReorg, wantBoundary: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &coherentStateReaderStub{gasPrices: test.gasPrices}
			source := &coherentStateSource{
				heads:  &sequenceHeadReader{headers: []*ethtypes.Header{start, test.end}},
				reader: reader,
			}

			snapshot, err := source.Snapshot(t.Context())
			if test.wantBoundary {
				if !errors.Is(err, errStateRefreshBlockBoundary) {
					t.Fatalf("Snapshot() error = %v, want block-boundary error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Snapshot(): %v", err)
			}
			if snapshot.GasLimit != start.GasLimit {
				t.Fatalf("gas limit = %d, want %d", snapshot.GasLimit, start.GasLimit)
			}
			if reader.gasReads != 1 {
				t.Fatalf("gas reads = %d, want 1", reader.gasReads)
			}
		})
	}
}

func TestReadHeadRejectsMissingNumber(t *testing.T) {
	_, err := readHead(t.Context(), &sequenceHeadReader{headers: []*ethtypes.Header{{}}})
	if err == nil {
		t.Fatal("readHead() accepted a header without a number")
	}
}

type sequenceHeadReader struct {
	headers []*ethtypes.Header
	next    int
}

func (r *sequenceHeadReader) HeaderByNumber(
	ctx context.Context,
	_ *big.Int,
) (*ethtypes.Header, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if r.next >= len(r.headers) {
		return nil, errors.New("unexpected latest-head read")
	}
	header := r.headers[r.next]
	r.next++
	return header, nil
}

type coherentStateReaderStub struct {
	gasReads  int
	gasPrices *liquidlanegas.PriceSnapshot
}

func (r *coherentStateReaderStub) ReadExecutorState(
	context.Context,
	common.Address,
	common.Address,
) (ExecutorState, error) {
	return ExecutorState{Nonce: big.NewInt(1), Deposit: new(big.Int).Set(minDeposit)}, nil
}

func (r *coherentStateReaderStub) ReadAdapterSnapshot(
	context.Context,
	common.Address,
	common.Address,
) (strategytypes.AdapterSnapshot, error) {
	return strategytypes.AdapterSnapshot{Loan: common.HexToAddress("0x1234")}, nil
}

func (r *coherentStateReaderStub) ReadGasPrices(
	_ context.Context,
	_ strategytypes.AdapterSnapshot,
) (*liquidlanegas.PriceSnapshot, error) {
	r.gasReads++
	return r.gasPrices, nil
}

func stateTestHeader(number int64, marker byte) *ethtypes.Header {
	return &ethtypes.Header{
		ParentHash:  common.BytesToHash([]byte{marker}),
		UncleHash:   ethtypes.EmptyUncleHash,
		Root:        common.BytesToHash([]byte{marker}),
		TxHash:      ethtypes.EmptyTxsHash,
		ReceiptHash: ethtypes.EmptyReceiptsHash,
		Difficulty:  big.NewInt(1),
		Number:      big.NewInt(number),
		GasLimit:    30_000_000,
		Time:        1_700_000_000,
		BaseFee:     big.NewInt(1),
	}
}
