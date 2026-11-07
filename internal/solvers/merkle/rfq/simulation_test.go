package rfq

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

func TestOuterFillProbeRequiresExactTaggedResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		edit    func([]byte) []byte
		fail    error
		success bool
		want    string
	}{
		{name: "authenticated original results"},
		{name: "probe returned success", success: true, want: "authenticated"},
		{name: "probe transport failed", fail: errors.New("RPC unavailable"), want: "authenticated"},
		{name: "unrelated revert", edit: func([]byte) []byte { return []byte{1, 2, 3, 4} }, want: "unexpected revert"},
		{name: "truncated result", edit: func(raw []byte) []byte { return raw[:10] }, want: "malformed"},
		{name: "trailing result", edit: func(raw []byte) []byte { return append(raw, make([]byte, 32)...) }, want: "identity"},
		{name: "wrong executor", edit: editProbeIdentity(t, 0), want: "identity"},
		{name: "wrong caller", edit: editProbeIdentity(t, 1), want: "identity"},
		{name: "wrong calldata", edit: editProbeIdentity(t, 2), want: "identity"},
		{name: "wrong runtime", edit: editProbeIdentity(t, 3), want: "identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := connectorExecutionFixture(t, connectorDiscountFixture())
			backend := &editedProbeReader{connectorExecutionReader: f.reader, edit: tc.edit, fail: tc.fail, success: tc.success}
			inner := []byte{1, 2, 3}
			reader := outerFillReader{Reader: backend, snapshot: recipes.Snapshot{Executor: f.reader.recipeExecutor}, inner: inner,
				outer: executorBinding.GetABI().Methods["fill"].ID, caller: f.workflow.caller, signer: f.workflow.signer}
			got, err := reader.CallContract(t.Context(), ethereum.CallMsg{From: reader.caller, To: &reader.snapshot.Executor, Data: inner, Gas: f.reader.header.GasLimit}, big.NewInt(7))
			if tc.want != "" {
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("err=%v want %s", err, tc.want)
				}
			} else if err != nil || len(got) == 0 {
				t.Fatalf("original execution results missing: %x %v", got, err)
			}
			if f.reader.outerCalls != 1 || backend.probes != 1 {
				t.Fatal("result probe bypassed the complete unchanged settlement")
			}
		})
	}
}

func editProbeIdentity(t *testing.T, index int) func([]byte) []byte {
	t.Helper()
	var artifact struct {
		ABI json.RawMessage `json:"abi"`
	}
	if err := json.Unmarshal(outerSimulationArtifact, &artifact); err != nil {
		t.Fatal(err)
	}
	binding, err := abi.JSON(bytes.NewReader(artifact.ABI))
	if err != nil {
		t.Fatal(err)
	}
	resultError := binding.Errors["RFQRecipeResult"]
	return func(raw []byte) []byte {
		values, err := resultError.Inputs.Unpack(raw[4:])
		if err != nil {
			t.Fatal(err)
		}
		if index < 2 {
			values[index] = common.HexToAddress("0xbad")
		} else {
			values[index] = [32]byte(common.HexToHash("0xbad"))
		}
		changed, err := resultError.Inputs.Pack(values...)
		if err != nil {
			t.Fatal(err)
		}
		return append(resultError.ID.Bytes()[:4], changed...)
	}
}

type editedProbeReader struct {
	*connectorExecutionReader

	edit    func([]byte) []byte
	fail    error
	success bool
	probes  int
}

func (r *editedProbeReader) CallContractWithCodeAt(ctx context.Context, msg ethereum.CallMsg, block *big.Int, target common.Address, code []byte) ([]byte, error) {
	r.probes++
	if r.success {
		return nil, nil
	}
	if r.fail != nil {
		return nil, r.fail
	}
	result, err := r.connectorExecutionReader.CallContractWithCodeAt(ctx, msg, block, target, code)
	if r.edit == nil {
		return result, err
	}
	var tagged recipeProbeError
	if !errors.As(err, &tagged) {
		return result, err
	}
	raw, err := hexutil.Decode(tagged.data)
	if err != nil {
		return nil, err
	}
	return nil, recipeProbeError{hexutil.Encode(r.edit(raw))}
}

func TestOuterFillProbeCannotReplaceCanonicalSettlement(t *testing.T) {
	f := connectorExecutionFixture(t, connectorDiscountFixture())
	f.reader.outerErr = errors.New("canonical Reactor settlement rejected")
	backend := &editedProbeReader{connectorExecutionReader: f.reader}
	inner := []byte{1}
	reader := outerFillReader{Reader: backend, snapshot: recipes.Snapshot{Executor: f.reader.recipeExecutor}, inner: inner,
		outer: executorBinding.GetABI().Methods["fill"].ID, caller: f.workflow.caller, signer: f.workflow.signer}
	if _, err := reader.CallContract(t.Context(), ethereum.CallMsg{From: reader.caller, To: &reader.snapshot.Executor, Data: inner, Gas: f.reader.header.GasLimit}, big.NewInt(7)); !errors.Is(err, f.reader.outerErr) || backend.probes != 0 {
		t.Fatalf("probe bypassed canonical settlement failure: probes=%d err=%v", backend.probes, err)
	}
	f.reader.outerErr = nil
	reader.Reader = struct{ recipes.Reader }{f.reader} // Ordinary eth_call lacks the nested return probe.
	if _, err := reader.CallContract(t.Context(), ethereum.CallMsg{From: reader.caller, To: &reader.snapshot.Executor, Data: inner, Gas: f.reader.header.GasLimit}, big.NewInt(7)); err == nil || !strings.Contains(err.Error(), "nested code-override") {
		t.Fatalf("unsupported nested simulation did not fail closed: %v", err)
	}
}
