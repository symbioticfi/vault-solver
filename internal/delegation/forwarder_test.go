package delegation

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/solverdelegate"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

type codeBackend struct {
	code map[common.Address][]byte
	err  error
}

func (b *codeBackend) CodeAt(_ context.Context, account common.Address, _ *big.Int) ([]byte, error) {
	return b.code[account], b.err
}

type fixtureAccounts struct {
	authority common.Address
	delegate  common.Address
	auxiliary common.Address
}

func forwarderFixture(t *testing.T) (*Forwarder, *codeBackend, fixtureAccounts) {
	t.Helper()
	authority := common.HexToAddress("0x1000")
	delegate := common.HexToAddress("0x2000")
	auxiliary := common.HexToAddress("0x3000")
	var artifact compilerArtifact
	if err := json.Unmarshal([]byte(solverdelegate.Artifact), &artifact); err != nil {
		t.Fatal(err)
	}
	code := hexutil.MustDecode(artifact.DeployedBytecode.Object)
	// The first immutable's offset is taken from the upstream solc artifact, not
	// from the production validator. Remaining approved callers are all zero.
	copy(code[40:60], auxiliary.Bytes())
	backend := &codeBackend{code: map[common.Address][]byte{
		authority: append([]byte{0xef, 0x01, 0x00}, delegate.Bytes()...), delegate: code,
	}}
	f, err := New(backend, authority, delegate, []common.Address{auxiliary})
	if err != nil {
		t.Fatal(err)
	}
	return f, backend, fixtureAccounts{authority: authority, delegate: delegate, auxiliary: auxiliary}
}

func TestForwarderValidatesDelegation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*codeBackend, common.Address, common.Address, common.Address)
		valid  bool
	}{
		{"valid", func(*codeBackend, common.Address, common.Address, common.Address) {}, true},
		{"not delegated", func(b *codeBackend, a, _, _ common.Address) { delete(b.code, a) }, false},
		{"wrong delegate", func(b *codeBackend, a, _, _ common.Address) { b.code[a][22] ^= 1 }, false},
		{"missing implementation", func(b *codeBackend, _, d, _ common.Address) { delete(b.code, d) }, false},
		{"changed runtime", func(b *codeBackend, _, d, _ common.Address) { b.code[d][0] ^= 1 }, false},
		{"unauthorized auxiliary", func(b *codeBackend, _, d, _ common.Address) { b.code[d][59] ^= 1 }, false},
		{"unknown approved caller", func(b *codeBackend, _, d, _ common.Address) { b.code[d][122] = 1 }, false},
		{"delegated auxiliary", func(b *codeBackend, _, _, a common.Address) { b.code[a] = []byte{1} }, false},
		{"RPC error", func(b *codeBackend, _, _, _ common.Address) { b.err = errors.New("unavailable") }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, b, accounts := forwarderFixture(t)
			tc.mutate(b, accounts.authority, accounts.delegate, accounts.auxiliary)
			if err := f.Validate(t.Context()); (err == nil) != tc.valid {
				t.Fatalf("Validate = %v, valid = %v", err, tc.valid)
			}
		})
	}
}

func TestForwarderPacksCallAndRejectsRevocation(t *testing.T) {
	f, b, accounts := forwarderFixture(t)
	authority := accounts.authority
	to := common.HexToAddress("0x4444444444444444444444444444444444444444")
	req := txmanager.Request{To: to, Data: []byte{0xaa, 0xbb}, Value: big.NewInt(123), GasLimit: 21_000}
	got, err := f.Prepare(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if got.To != authority || !bytes.Equal(got.Data, hexutil.MustDecode("0x4444444444444444444444444444444444444444aabb")) ||
		got.Value.Cmp(big.NewInt(123)) != 0 || got.GasLimit != 0 {
		t.Fatalf("wrong outer call: %+v", got)
	}
	if req.To != to || !bytes.Equal(req.Data, []byte{0xaa, 0xbb}) {
		t.Fatal("modified the caller's request")
	}
	delete(b.code, authority)
	if _, err := f.Prepare(t.Context(), req); err == nil {
		t.Fatal("revoked delegation must not produce an empty EOA transfer")
	}
}

func TestForwarderRejectsInvalidCallerSets(t *testing.T) {
	a := common.HexToAddress("0x1000")
	d := common.HexToAddress("0x2000")
	x := common.HexToAddress("0x3000")
	for _, callers := range [][]common.Address{nil, {a}, {{}}, {x, x}, {x, x, x, x, x, x}} {
		if _, err := New(&codeBackend{}, a, d, callers); err == nil {
			t.Fatalf("accepted callers %v", callers)
		}
	}
}
