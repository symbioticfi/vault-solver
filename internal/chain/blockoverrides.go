package chain

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/rpc"
)

// JSON-RPC error codes the classifiers below rely on.
const (
	jsonRPCCodeExecutionReverted = 3      // geth: a call that reverted, with its revert data
	jsonRPCCodeResourceNotFound  = -32001 // EIP-1474: the requested resource (here, a block) is unknown
	jsonRPCCodeMethodNotFound    = -32601
	jsonRPCCodeInvalidParams     = -32602
)

// blockOverridesProbeTarget is the empty account the probe installs its code at through a state
// override. It is neither a precompile nor a deployed contract, and nothing is ever sent to it.
var blockOverridesProbeTarget = common.HexToAddress("0x00000000000000000000000000000000000c0de1")

// NextBlockOverrides is the header a next-block gas estimate on top of head (which must carry a
// number) runs under: the next number, and a timestamp one block time later, rounded up to whole
// seconds so it always advances by at least one second.
func NextBlockOverrides(head *types.Header, blockTime time.Duration) ethereum.BlockOverrides {
	step := uint64(1)
	if blockTime > time.Second {
		step = uint64((blockTime + time.Second - 1) / time.Second)
	}
	return ethereum.BlockOverrides{
		Number: new(big.Int).Add(head.Number, big.NewInt(1)),
		Time:   head.Time + step,
	}
}

// ProbeBlockOverrides reports whether the read endpoint honours eth_estimateGas block overrides. It
// estimates a call to code, installed by a state override, that stops only when NUMBER and TIMESTAMP
// equal the NextBlockOverrides of the latest head and reverts otherwise. An upstream that silently
// ignores the fourth parameter executes on the head's own header and reverts, which the error-based
// IsBlockOverridesUnsupported can never notice. One that silently ignores the state override calls
// an empty account instead, which costs exactly the intrinsic 21000 gas, so success only counts when
// the estimate shows the code ran. False with a nil error means overrides are unsupported (rejected
// or ignored); an error means the probe was inconclusive.
func (c *Client) ProbeBlockOverrides(ctx context.Context, blockTime time.Duration) (bool, error) {
	if blockTime <= 0 {
		return false, errors.New("chain: block overrides probe needs a positive block time")
	}
	head, err := c.HeaderByNumber(ctx, nil)
	if err != nil {
		return false, errors.Errorf("chain: block overrides probe head: %w", err)
	}
	if head == nil || head.Number == nil || !head.Number.IsUint64() || head.Number.Uint64() == math.MaxUint64 {
		return false, errors.New("chain: block overrides probe head has no usable block number")
	}
	overrides := NextBlockOverrides(head, blockTime)
	target := blockOverridesProbeTarget
	state := map[common.Address]ethereum.OverrideAccount{
		target: {Code: blockOverridesProbeCode(overrides.Number.Uint64(), overrides.Time)},
	}
	gas, err := c.estimateGasWithOverrides(ctx, ethereum.CallMsg{To: &target}, head.Number, state, overrides)
	switch {
	case err == nil:
		return gas > params.TxGas, nil
	case IsBlockOverridesUnsupported(err), IsExecutionReverted(err):
		return false, nil
	default:
		return false, errors.Errorf("chain: block overrides probe: %w", err)
	}
}

// EVM opcodes used by blockOverridesProbeCode.
const (
	opStop      = 0x00
	opEq        = 0x14
	opAnd       = 0x16
	opTimestamp = 0x42
	opNumber    = 0x43
	opJumpi     = 0x57
	opJumpdest  = 0x5b
	opPush1     = 0x60
	opPush8     = 0x67
	opRevert    = 0xfd
)

// blockOverridesProbeCode assembles
//
//	NUMBER PUSH8 number EQ TIMESTAMP PUSH8 time EQ AND PUSH1 ok JUMPI
//	PUSH1 0 PUSH1 0 REVERT
//	ok: JUMPDEST STOP
//
// so the call succeeds only in a block with exactly that number and timestamp.
func blockOverridesProbeCode(number, time uint64) []byte {
	code := []byte{opNumber, opPush8}
	code = binary.BigEndian.AppendUint64(code, number)
	code = append(code, opEq, opTimestamp, opPush8)
	code = binary.BigEndian.AppendUint64(code, time)
	code = append(code, opEq, opAnd, opPush1, 0, opJumpi)
	okOffset := len(code) - 2
	code = append(code, opPush1, 0, opPush1, 0, opRevert)
	code[okOffset] = byte(len(code))
	return append(code, opJumpdest, opStop)
}

// IsBlockOverridesUnsupported reports whether err is an upstream refusing the eth_estimateGas block
// overrides parameter: an invalid-params error (-32602), or a message about too many or invalid
// arguments, whether the node answered in a JSON-RPC error or in the body of a non-2xx response. A
// revert, or a node that lacks the parent block (anvil reports that as -32602 too), is never
// classified as unsupported.
func IsBlockOverridesUnsupported(err error) bool {
	if err == nil || IsExecutionReverted(err) || IsBlockNotFound(err) {
		return false
	}
	code, message, ok := jsonRPCError(err)
	if !ok {
		return false
	}
	if code == jsonRPCCodeInvalidParams {
		return true
	}
	if code == jsonRPCCodeMethodNotFound {
		return false
	}
	message = strings.ToLower(message)
	return strings.Contains(message, "too many arguments") ||
		strings.Contains(message, "invalid params") ||
		strings.Contains(message, "invalid argument")
}

// IsExecutionReverted reports whether err is a node reporting that the simulated call reverted.
func IsExecutionReverted(err error) bool {
	code, message, ok := jsonRPCError(err)
	if !ok {
		return false
	}
	return code == jsonRPCCodeExecutionReverted || strings.Contains(strings.ToLower(message), "execution reverted")
}

// jsonRPCErrorEnvelope decodes the error member of a JSON-RPC response body.
type jsonRPCErrorEnvelope struct {
	Error *jsonRPCErrorObject `json:"error"`
}

type jsonRPCErrorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// jsonRPCError extracts the error a node answered with: the client's decoded JSON-RPC error, or the
// body of a non-2xx HTTP response, which the client does not decode. Transport failures report !ok.
func jsonRPCError(err error) (code int, message string, ok bool) {
	var rpcErr rpc.Error
	if errors.As(err, &rpcErr) {
		return rpcErr.ErrorCode(), rpcErr.Error(), true
	}
	var httpErr rpc.HTTPError
	if !errors.As(err, &httpErr) || len(httpErr.Body) == 0 {
		return 0, "", false
	}
	var envelope jsonRPCErrorEnvelope
	if json.Unmarshal(httpErr.Body, &envelope) == nil && envelope.Error != nil {
		return envelope.Error.Code, envelope.Error.Message, true
	}
	return 0, string(httpErr.Body), true
}

// blockNotFoundPhrases are how nodes word a missing block: geth ("header not found", "header for hash
// not found"), erigon and others ("block not found", "unknown block"), nethermind ("could not be
// found") and anvil ("BlockOutOfRangeError", sent with -32602). geth's "hash is not currently
// canonical" answers an EIP-1898 hash read with requireCanonical after a reorg replaced that block:
// the pin is as stale as one the node has not imported yet, so it is classified the same way.
var blockNotFoundPhrases = []string{
	"header not found", "header for hash not found", "block not found", "unknown block",
	"could not be found", "blockoutofrange", "not currently canonical",
}

// IsBlockNotFound reports whether err is a node saying it does not have the requested block, the
// usual answer of an upstream that has not imported the pinned block yet, or no longer has it on
// its canonical chain. A read pinned to a block another upstream reported (a pinned balance, a
// next-block gas estimate on top of a header) retries it rather than failing.
func IsBlockNotFound(err error) bool {
	code, message, ok := jsonRPCError(err)
	if !ok || code == jsonRPCCodeMethodNotFound {
		return false
	}
	if code == jsonRPCCodeResourceNotFound {
		return true
	}
	message = strings.ToLower(message)
	for _, phrase := range blockNotFoundPhrases {
		if strings.Contains(message, phrase) {
			return true
		}
	}
	return false
}

// pinnedBlockParam encodes an explicit block reference: a hash as an EIP-1898 object (keeping
// requireCanonical) and a number as a plain quantity, the form every node accepts. Tags such as
// latest, and references that set both or neither, are refused.
func pinnedBlockParam(block rpc.BlockNumberOrHash) (any, error) {
	hash, byHash := block.Hash()
	number, byNumber := block.Number()
	switch {
	case byHash && !byNumber:
		return rpc.BlockNumberOrHashWithHash(hash, block.RequireCanonical), nil
	case byNumber && !byHash && number >= 0:
		return hexutil.Uint64(number), nil
	default:
		return nil, errors.Errorf("chain: a pinned read needs exactly one explicit block hash or number, got %s", block.String())
	}
}

// callArg encodes a call message exactly as ethclient does, for the raw calls it has no method for.
func callArg(msg ethereum.CallMsg) map[string]any {
	arg := map[string]any{
		"from": msg.From,
		"to":   msg.To,
	}
	if len(msg.Data) > 0 {
		arg["input"] = hexutil.Bytes(msg.Data)
	}
	if msg.Value != nil {
		arg["value"] = (*hexutil.Big)(msg.Value)
	}
	if msg.Gas != 0 {
		arg["gas"] = hexutil.Uint64(msg.Gas)
	}
	if msg.GasPrice != nil {
		arg["gasPrice"] = (*hexutil.Big)(msg.GasPrice)
	}
	if msg.GasFeeCap != nil {
		arg["maxFeePerGas"] = (*hexutil.Big)(msg.GasFeeCap)
	}
	if msg.GasTipCap != nil {
		arg["maxPriorityFeePerGas"] = (*hexutil.Big)(msg.GasTipCap)
	}
	if msg.AccessList != nil {
		arg["accessList"] = msg.AccessList
	}
	if msg.BlobGasFeeCap != nil {
		arg["maxFeePerBlobGas"] = (*hexutil.Big)(msg.BlobGasFeeCap)
	}
	if msg.BlobHashes != nil {
		arg["blobVersionedHashes"] = msg.BlobHashes
	}
	if msg.AuthorizationList != nil {
		arg["authorizationList"] = msg.AuthorizationList
	}
	return arg
}
