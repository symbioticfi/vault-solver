package chain

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

const testErrorsABI = `[
 {"type":"error","name":"InsufficientBalance","inputs":[{"name":"balance","type":"uint256"},{"name":"needed","type":"uint256"}]},
 {"type":"error","name":"InvalidProtocolSignature","inputs":[]}
]`

type dataRPCError struct{ data any }

func (e dataRPCError) Error() string  { return "execution reverted" }
func (e dataRPCError) ErrorCode() int { return 3 }
func (e dataRPCError) ErrorData() any { return e.data }

type carrierError struct{ data []byte }

func (e carrierError) Error() string      { return "relay simulation reverted" }
func (e carrierError) RevertData() []byte { return e.data }

func encodeRevert(t *testing.T, signature string, args abi.Arguments, values ...any) []byte {
	t.Helper()
	packed, err := args.Pack(values...)
	if err != nil {
		t.Fatal(err)
	}
	return append(crypto.Keccak256([]byte(signature))[:4], packed...)
}

func TestRevertDecoderDescribe(t *testing.T) {
	t.Parallel()
	parsed, err := abi.JSON(strings.NewReader(testErrorsABI))
	if err != nil {
		t.Fatal(err)
	}
	decoder := NewRevertDecoder(&parsed)
	uint256, _ := abi.NewType("uint256", "", nil)
	str, _ := abi.NewType("string", "", nil)
	for name, tc := range map[string]struct {
		data []byte
		want string
	}{
		"custom error with arguments": {
			encodeRevert(t, "InsufficientBalance(uint256,uint256)", abi.Arguments{{Type: uint256}, {Type: uint256}}, big.NewInt(5), big.NewInt(9)),
			"InsufficientBalance(balance=5, needed=9)",
		},
		"custom error without arguments": {crypto.Keccak256([]byte("InvalidProtocolSignature()"))[:4], "InvalidProtocolSignature()"},
		"Error(string)": {
			encodeRevert(t, "Error(string)", abi.Arguments{{Type: str}}, "ERC20: insufficient allowance"),
			`"ERC20: insufficient allowance"`,
		},
		"Panic(uint256)": {
			encodeRevert(t, "Panic(uint256)", abi.Arguments{{Type: uint256}}, big.NewInt(0x11)),
			"panic: arithmetic underflow or overflow",
		},
		"unknown selector":      {hexutil.MustDecode("0xdeadbeef00"), "unknown error 0xdeadbeef (5 bytes)"},
		"undecodable arguments": {crypto.Keccak256([]byte("InsufficientBalance(uint256,uint256)"))[:4], "InsufficientBalance (undecodable arguments)"},
		"short payload":         {[]byte{1, 2}, "malformed revert data 0x0102"},
		"empty":                 {nil, "no revert data"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := decoder.Describe(tc.data); got != tc.want {
				t.Fatalf("Describe = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRevertDataAndExplain(t *testing.T) {
	t.Parallel()
	payload := crypto.Keccak256([]byte("InvalidProtocolSignature()"))[:4]
	parsed, _ := abi.JSON(strings.NewReader(testErrorsABI))
	decoder := NewRevertDecoder(&parsed)

	rpcRevert := errors.Errorf("simulate fill: %w", dataRPCError{data: hexutil.Encode(payload)})
	if data, reverted := RevertData(rpcRevert); !reverted || hexutil.Encode(data) != hexutil.Encode(payload) {
		t.Fatalf("rpc revert = %x, %t", data, reverted)
	}
	if got := decoder.Explain(rpcRevert).Error(); got != "simulate fill: execution reverted (revert InvalidProtocolSignature())" {
		t.Fatalf("Explain = %q", got)
	}
	if !errors.Is(decoder.Explain(rpcRevert), rpcRevert) {
		t.Fatal("Explain dropped the original error")
	}
	if data, reverted := RevertData(carrierError{data: payload}); !reverted || len(data) != 4 {
		t.Fatalf("carrier revert = %x, %t", data, reverted)
	}
	// A revert the node reported without a payload keeps its own message.
	bare := errors.New("execution reverted: ERC20: insufficient allowance")
	if _, reverted := RevertData(bare); !reverted || !errors.Is(decoder.Explain(bare), bare) {
		t.Fatal("bare revert should be recognized and returned unchanged")
	}
	transport := errors.New("context deadline exceeded")
	if _, reverted := RevertData(transport); reverted || !errors.Is(decoder.Explain(transport), transport) {
		t.Fatal("transport error must not be treated as a revert")
	}
	if _, reverted := RevertData(nil); reverted {
		t.Fatal("nil is not a revert")
	}
}

type fakeReplayer struct {
	err   error
	block *big.Int
}

func (f *fakeReplayer) ReplayMinedCall(_ context.Context, _ ethereum.CallMsg, block *big.Int) ([]byte, error) {
	f.block = block
	return nil, f.err
}

func TestMinedRevertReplaysTheMinedBlock(t *testing.T) {
	t.Parallel()
	parsed, _ := abi.JSON(strings.NewReader(testErrorsABI))
	decoder := NewRevertDecoder(&parsed)
	payload := hexutil.Encode(crypto.Keccak256([]byte("InvalidProtocolSignature()"))[:4])
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"decoded":        {dataRPCError{data: payload}, "replay of block 100 reverted with InvalidProtocolSignature()"},
		"no payload":     {errors.New("execution reverted: Ownable: caller is not the owner"), "replay of block 100 reverted: execution reverted: Ownable: caller is not the owner"},
		"did not revert": {nil, "replay of block 100 did not revert (an earlier transaction in the block changed the state)"},
		"read failed":    {errors.New("context deadline exceeded"), "revert reason unavailable: replay of block 100 failed: context deadline exceeded"},
	} {
		t.Run(name, func(t *testing.T) {
			replayer := &fakeReplayer{err: tc.err}
			if got := decoder.MinedRevert(t.Context(), replayer, ethereum.CallMsg{}, big.NewInt(100)); got != tc.want {
				t.Fatalf("MinedRevert = %q, want %q", got, tc.want)
			}
			if replayer.block.Int64() != 100 {
				t.Fatalf("replayed block %s, want 100", replayer.block)
			}
		})
	}
	if got := decoder.MinedRevert(t.Context(), &fakeReplayer{}, ethereum.CallMsg{}, nil); got != "revert reason unavailable: unknown inclusion block" {
		t.Fatalf("nil block = %q", got)
	}
}
