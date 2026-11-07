package rfq

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

//go:embed simulation/OuterFillSimulation.json
var outerSimulationArtifact []byte

type outerSimulationBackend interface {
	recipes.Reader
	CodeAt(ctx context.Context, address common.Address, block *big.Int) ([]byte, error)
	CallContractWithCodeAt(ctx context.Context, msg ethereum.CallMsg, block *big.Int, target common.Address, code []byte) ([]byte, error)
}

// The SDK admits the immutable inner recipe using values from the complete RFQ
// funding context. Only the separate unchanged outer eth_call proves settlement.
type outerFillReader struct {
	recipes.Reader

	snapshot       recipes.Snapshot
	inner, outer   []byte
	caller, signer common.Address
}

func (r outerFillReader) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if msg.To == nil || *msg.To != r.snapshot.Executor || msg.From != r.caller || !bytes.Equal(msg.Data, r.inner) {
		return r.Reader.CallContract(ctx, msg, block)
	}
	if block == nil || block.Sign() < 0 || msg.Gas == 0 || r.signer == (common.Address{}) {
		return nil, errors.New("outer RFQ simulation requires signer, pinned block and gas budget")
	}
	outer := ethereum.CallMsg{From: r.signer, To: &r.caller, Data: r.outer, Gas: msg.Gas}
	if _, err := r.Reader.CallContract(ctx, outer, block); err != nil {
		return nil, err
	}
	backend, ok := r.Reader.(outerSimulationBackend)
	if !ok {
		return nil, errors.New("outer RFQ simulation requires nested code-override support")
	}
	code, err := backend.CodeAt(ctx, r.snapshot.Executor, block)
	if err != nil {
		return nil, errors.Errorf("read original RFQ recipe runtime: %w", err)
	}
	if len(code) == 0 || len(code) > 24576 {
		return nil, errors.New("invalid original RFQ recipe runtime size")
	}
	var artifact struct {
		Runtime hexutil.Bytes   `json:"runtime"`
		ABI     json.RawMessage `json:"abi"`
	}
	if err := json.Unmarshal(outerSimulationArtifact, &artifact); err != nil || len(artifact.Runtime) == 0 {
		return nil, errors.New("invalid outer RFQ simulation artifact")
	}
	binding, err := abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		return nil, errors.Errorf("decode outer RFQ simulation ABI: %w", err)
	}
	resultError := binding.Errors["RFQRecipeResult"]
	calldataHash, runtimeHash := crypto.Keccak256Hash(r.inner), crypto.Keccak256Hash(code)
	configuration, err := (abi.Arguments{resultError.Inputs[4], resultError.Inputs[1], resultError.Inputs[2]}).Pack(code, r.caller, calldataHash)
	if err != nil {
		return nil, errors.Errorf("encode outer RFQ simulation configuration: %w", err)
	}
	override := append(bytes.Clone(artifact.Runtime), configuration...)
	override = append(override, common.LeftPadBytes(new(big.Int).SetUint64(uint64(len(configuration))).Bytes(), 32)...)
	// Runtime-copy creation has no place in the real transaction's gas budget.
	const overhead = uint64(8_000_000)
	if outer.Gas > ^uint64(0)-overhead {
		return nil, errors.New("outer RFQ simulation gas budget overflows probe allowance")
	}
	outer.Gas += overhead
	_, probeErr := backend.CallContractWithCodeAt(ctx, outer, block, r.snapshot.Executor, override)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var dataError rpc.DataError
	if !errors.As(probeErr, &dataError) {
		return nil, errors.New("outer RFQ simulation did not return authenticated recipe values")
	}
	encoded, ok := dataError.ErrorData().(string)
	if !ok {
		return nil, errors.New("outer RFQ simulation returned invalid revert data")
	}
	raw, err := hexutil.Decode(encoded)
	if err != nil || len(raw) < 4 || !bytes.Equal(raw[:4], resultError.ID.Bytes()[:4]) {
		return nil, errors.New("outer RFQ simulation returned an unexpected revert")
	}
	values, err := resultError.Inputs.Unpack(raw[4:])
	if err != nil {
		return nil, errors.New("outer RFQ simulation returned malformed recipe values")
	}
	canonical, err := resultError.Inputs.Pack(values...)
	if err != nil || !bytes.Equal(canonical, raw[4:]) || values[0] != r.snapshot.Executor || values[1] != r.caller || values[2] != [32]byte(calldataHash) || values[3] != [32]byte(runtimeHash) {
		return nil, errors.New("outer RFQ simulation recipe identity mismatch")
	}
	return bytes.Clone(values[4].([]byte)), nil
}
