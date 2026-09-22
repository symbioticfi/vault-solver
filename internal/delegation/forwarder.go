// Package delegation adapts the pinned Solver7702Delegate packed-call transport.
// It contains the external contract details; the transaction framework only sees
// a request preparer and independent sending accounts.
package delegation

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/bindings/solverdelegate"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// CodeReader reads public account bytecode. All reads must honor cancellation.
type CodeReader interface {
	CodeAt(ctx context.Context, account common.Address, block *big.Int) ([]byte, error)
}

// Forwarder is immutable after construction and safe to share across sender workers.
type Forwarder struct {
	backend   CodeReader
	authority common.Address
	delegate  common.Address
	callers   []common.Address
	runtime   []byte
	slots     []immutableReference
}

type immutableReference struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

type compilerArtifact struct {
	Bytecode         bytecodeArtifact `json:"bytecode"`
	DeployedBytecode bytecodeArtifact `json:"deployedBytecode"`
}

type bytecodeArtifact struct {
	Object              string                          `json:"object"`
	ImmutableReferences map[string][]immutableReference `json:"immutableReferences"`
}

// New builds an adapter for an already installed delegation. It never signs an
// authorization or deploys a contract. Validate verifies the live setup at startup.
func New(backend CodeReader, authority, delegate common.Address, callers []common.Address) (*Forwarder, error) {
	if backend == nil || authority == (common.Address{}) || delegate == (common.Address{}) || authority == delegate {
		return nil, errors.New("delegation: reader and distinct nonzero authority/delegate addresses are required")
	}
	if len(callers) < 1 || len(callers) > 5 {
		return nil, errors.New("delegation: requires one to five auxiliary callers")
	}
	seen := make(map[common.Address]bool, len(callers))
	for _, caller := range callers {
		if caller == (common.Address{}) || caller == authority || caller == delegate || seen[caller] {
			return nil, errors.New("delegation: callers must be unique nonzero accounts distinct from authority and delegate")
		}
		seen[caller] = true
	}
	var artifact compilerArtifact
	if err := json.Unmarshal([]byte(solverdelegate.Artifact), &artifact); err != nil {
		return nil, errors.Errorf("delegation: decode vendored runtime: %w", err)
	}
	runtime, err := hexutil.Decode(artifact.DeployedBytecode.Object)
	if err != nil {
		return nil, errors.Errorf("delegation: decode runtime bytecode: %w", err)
	}
	f := &Forwarder{backend: backend, authority: authority, delegate: delegate, callers: slices.Clone(callers), runtime: runtime}
	for _, refs := range artifact.DeployedBytecode.ImmutableReferences {
		for _, ref := range refs {
			if ref.Length != 32 || ref.Start < 0 || ref.Start > len(runtime)-32 {
				return nil, errors.New("delegation: invalid vendored immutable reference")
			}
			f.slots = append(f.slots, ref)
		}
	}
	if len(f.slots) != 5 {
		return nil, errors.New("delegation: vendored runtime must have five caller slots")
	}
	return f, nil
}

// Validate fails closed unless the authority delegates to the configured
// implementation, its bytecode matches the pinned compiler artifact, and its
// complete immutable caller set matches the configured auxiliary accounts.
func (f *Forwarder) Validate(ctx context.Context) error {
	if err := f.checkAuthority(ctx); err != nil {
		return err
	}
	code, err := f.backend.CodeAt(ctx, f.delegate, nil)
	if err != nil {
		return errors.Errorf("delegation: read implementation code: %w", err)
	}
	if err := f.validateRuntime(code); err != nil {
		return err
	}
	for _, caller := range f.callers {
		auxiliaryCode, err := f.backend.CodeAt(ctx, caller, nil)
		if err != nil {
			return errors.Errorf("delegation: read auxiliary code: %w", err)
		}
		if len(auxiliaryCode) != 0 {
			return errors.Errorf("delegation: auxiliary %s must be an undelegated EOA", caller)
		}
	}
	return nil
}

func (f *Forwarder) validateRuntime(code []byte) error {
	if len(code) != len(f.runtime) {
		return errors.New("delegation: implementation does not match pinned runtime")
	}
	normalized := slices.Clone(code)
	seen := make(map[common.Address]bool, len(f.callers))
	for _, slot := range f.slots {
		word := normalized[slot.Start : slot.Start+slot.Length]
		if !bytes.Equal(word[:12], make([]byte, 12)) {
			return errors.New("delegation: malformed approved caller")
		}
		caller := common.BytesToAddress(word)
		if caller != (common.Address{}) {
			if !slices.Contains(f.callers, caller) || seen[caller] {
				return errors.New("delegation: unexpected or duplicate approved caller")
			}
			seen[caller] = true
		}
		clear(word)
	}
	if !bytes.Equal(normalized, f.runtime) {
		return errors.New("delegation: implementation does not match pinned runtime")
	}
	if len(seen) != len(f.callers) {
		return errors.New("delegation: an auxiliary signer is not an approved caller")
	}
	return nil
}

func (f *Forwarder) checkAuthority(ctx context.Context) error {
	code, err := f.backend.CodeAt(ctx, f.authority, nil)
	if err != nil {
		return errors.Errorf("delegation: read authority code: %w", err)
	}
	want := append([]byte{0xef, 0x01, 0x00}, f.delegate.Bytes()...)
	if !bytes.Equal(code, want) {
		return errors.New("delegation: authority does not point to the configured delegate")
	}
	return nil
}

// Prepare preserves value and protocol deadlines, wraps only target/calldata,
// and forces estimation of the exact auxiliary -> authority -> target call.
// Re-checking the designator prevents an already revoked delegate from turning
// a fill into a successful but empty transfer to an ordinary EOA.
func (f *Forwarder) Prepare(ctx context.Context, req txmanager.Request) (txmanager.Request, error) {
	if err := f.checkAuthority(ctx); err != nil {
		return txmanager.Request{}, err
	}
	packed := make([]byte, common.AddressLength+len(req.Data))
	copy(packed, req.To.Bytes())
	copy(packed[common.AddressLength:], req.Data)
	req.To, req.Data, req.GasLimit = f.authority, packed, 0
	return req, nil
}
