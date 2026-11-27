package chain

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
)

const (
	maxRevertValueLen = 66
	// maxRevertArgsHexLen keeps the arguments of an undeclared error, e.g. a policy's amounts, bounded.
	maxRevertArgsHexLen = 258
)

// plainReverts names Error(string) and Panic payloads for chain-level reads that know no contract ABI.
var plainReverts = NewRevertDecoder()

// panicSelector is Panic(uint256), which abi.UnpackRevert decodes without saying it was a panic.
var panicSelector = [4]byte{0x4e, 0x48, 0x7b, 0x71}

// RevertData reports whether err is an execution revert from eth_call or eth_estimateGas and returns
// its payload. data is nil when the node omitted or malformed it; the node's message may still name
// the reason.
func RevertData(err error) (data []byte, reverted bool) {
	if err == nil {
		return nil, false
	}
	var carrier interface{ RevertData() []byte }
	if errors.As(err, &carrier) {
		return carrier.RevertData(), true
	}
	var rpcErr rpc.Error
	if (!errors.As(err, &rpcErr) || rpcErr.ErrorCode() != 3) &&
		!strings.Contains(strings.ToLower(err.Error()), "execution reverted") {
		return nil, false
	}
	var dataErr rpc.DataError
	if errors.As(err, &dataErr) {
		if encoded, ok := dataErr.ErrorData().(string); ok {
			if decoded, decodeErr := hexutil.Decode(encoded); decodeErr == nil {
				return decoded, true
			}
		}
	}
	return nil, true
}

// RevertDecoder names revert payloads using the custom errors of the ABIs it was built from, plus
// Error(string) and Panic(uint256). It knows no protocol; integrations pass their contracts' ABIs.
// Safe for concurrent use after construction.
type RevertDecoder struct {
	errs map[[4]byte]abi.Error
}

// RevertDecoderFor builds a decoder from generated binding metadata. The ABIs are static and
// vendored, so a parse failure is a programmer error.
func RevertDecoderFor(metas ...*bind.MetaData) *RevertDecoder {
	abis := make([]*abi.ABI, 0, len(metas))
	for _, meta := range metas {
		parsed, err := meta.ParseABI()
		if err != nil {
			panic(errors.Errorf("parse vendored ABI for revert decoding: %w", err))
		}
		abis = append(abis, parsed)
	}
	return NewRevertDecoder(abis...)
}

func NewRevertDecoder(abis ...*abi.ABI) *RevertDecoder {
	d := &RevertDecoder{errs: make(map[[4]byte]abi.Error)}
	for _, parsed := range abis {
		if parsed == nil {
			continue
		}
		for _, e := range parsed.Errors {
			d.errs[[4]byte(e.ID[:4])] = e
		}
	}
	return d
}

// Describe names a revert payload, e.g. `InvalidDiscount()`, `"ERC20: insufficient allowance"`,
// `panic: arithmetic underflow or overflow`, or the raw selector and arguments when no ABI declares it.
func (d *RevertDecoder) Describe(data []byte) string {
	switch {
	case len(data) == 0:
		return "no revert data"
	case len(data) < 4:
		return "malformed revert data " + hexutil.Encode(data)
	}
	if reason, err := abi.UnpackRevert(data); err == nil {
		if [4]byte(data[:4]) == panicSelector {
			return "panic: " + reason
		}
		return fmt.Sprintf("%q", reason)
	}
	if d != nil {
		if e, ok := d.errs[[4]byte(data[:4])]; ok {
			values, err := e.Inputs.Unpack(data[4:])
			if err != nil {
				return e.Name + " (undecodable arguments)"
			}
			return formatRevert(e, values)
		}
	}
	if len(data) == 4 {
		return fmt.Sprintf("unknown error %s (4 bytes)", hexutil.Encode(data))
	}
	return fmt.Sprintf("unknown error %s (%d bytes, args %s)", hexutil.Encode(data[:4]), len(data),
		truncate(hexutil.Encode(data[4:]), maxRevertArgsHexLen))
}

// Explain returns err with its decoded revert appended. A node's revert reply without a payload or
// reason is marked; other errors, including reverts a wrapper re-reported as text, are unchanged.
func (d *RevertDecoder) Explain(err error) error {
	data, reverted := RevertData(err)
	var rpcErr rpc.Error
	switch {
	case !reverted:
		return err
	case len(data) > 0:
		return errors.Errorf("%w (revert %s)", err, d.Describe(data))
	case strings.Contains(err.Error(), "execution reverted: "), !errors.As(err, &rpcErr) || rpcErr.ErrorCode() != 3:
		return err
	default:
		return errors.Errorf("%w (node returned no revert data)", err)
	}
}

func formatRevert(e abi.Error, values []any) string {
	parts := make([]string, len(values))
	for i, value := range values {
		name := fmt.Sprintf("arg%d", i)
		if i < len(e.Inputs) && e.Inputs[i].Name != "" {
			name = e.Inputs[i].Name
		}
		parts[i] = name + "=" + formatRevertValue(value)
	}
	return e.Name + "(" + strings.Join(parts, ", ") + ")"
}

func formatRevertValue(value any) string {
	switch v := value.(type) {
	case *big.Int:
		// At most 78 digits; a cut number would read as a different magnitude.
		return v.String()
	case common.Address:
		return v.Hex()
	case []byte:
		return truncate(hexutil.Encode(v), maxRevertValueLen)
	}
	if rv := reflect.ValueOf(value); rv.Kind() == reflect.Array && rv.Type().Elem().Kind() == reflect.Uint8 {
		b := make([]byte, rv.Len())
		reflect.Copy(reflect.ValueOf(b), rv)
		return truncate(hexutil.Encode(b), maxRevertValueLen)
	}
	return truncate(fmt.Sprintf("%v", value), maxRevertValueLen)
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// MinedCallReplayer re-executes a mined transaction's call; *Client implements it with ReplayMinedCall.
type MinedCallReplayer interface {
	ReplayMinedCall(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error)
}

const minedRevertReplayTimeout = 5 * time.Second

// MinedRevert describes why a mined transaction reverted by replaying its call (receipts carry no
// revert data). Best effort and bounded: the replay may succeed when an earlier transaction in the
// same block changed the state, and a failed read only says so. msg must carry the reverted
// transaction's sender, target, value and calldata. Call it only on the failure path.
func (d *RevertDecoder) MinedRevert(ctx context.Context, replayer MinedCallReplayer, msg ethereum.CallMsg, block *big.Int) string {
	if block == nil || block.Sign() <= 0 {
		return "revert reason unavailable: unknown inclusion block"
	}
	ctx, cancel := context.WithTimeout(ctx, minedRevertReplayTimeout)
	defer cancel()
	_, err := replayer.ReplayMinedCall(ctx, msg, block)
	if err == nil {
		return fmt.Sprintf("replay of block %s did not revert (state changed earlier in the block, or the tx ran out of gas)", block)
	}
	data, reverted := RevertData(err)
	if !reverted {
		return fmt.Sprintf("revert reason unavailable: replay of block %s failed: %v", block, err)
	}
	if len(data) == 0 {
		return fmt.Sprintf("replay of block %s reverted: %v", block, err)
	}
	return fmt.Sprintf("replay of block %s reverted with %s", block, d.Describe(data))
}
