package chain

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind/v2"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
)

const maxRevertValueLen = 66

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
// `panic: arithmetic underflow or overflow (0x11)`, or the raw selector when no ABI declares it.
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
	return fmt.Sprintf("unknown error %s (%d bytes)", hexutil.Encode(data[:4]), len(data))
}

// Explain returns err with its decoded revert appended, or err unchanged when it is not a revert
// or carries no payload (the node's message is then all there is).
func (d *RevertDecoder) Explain(err error) error {
	data, reverted := RevertData(err)
	if !reverted || len(data) == 0 {
		return err
	}
	return errors.Errorf("%w (revert %s)", err, d.Describe(data))
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
	var s string
	switch v := value.(type) {
	case *big.Int:
		s = v.String()
	case common.Address:
		s = v.Hex()
	case []byte:
		s = hexutil.Encode(v)
	case [32]byte:
		s = hexutil.Encode(v[:])
	default:
		s = fmt.Sprintf("%v", v)
	}
	if len(s) > maxRevertValueLen {
		s = s[:maxRevertValueLen] + "…"
	}
	return s
}
