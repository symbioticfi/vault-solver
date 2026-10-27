package rfq

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
)

// ABI offsets remain committed while a bounded signature occupies its reserved capacity.
// The generic recipe assembler still verifies every fixed bit and the complete leaf hash.
func fitConnectorSignatures(leaf recipes.Leaf, signed connector.ILiquidLaneConnectorDiscountSwap, protocol []byte, recipient common.Address, amount *big.Int) ([]byte, error) {
	if len(leaf.Data) != len(leaf.DataMask) || len(leaf.Data) < 420 {
		return nil, errors.New("connector signature layout is malformed")
	}
	word := func(at int) (int, error) {
		if at < 4 || at+32 > len(leaf.Data) {
			return 0, errors.New("connector signature offset is out of bounds")
		}
		n := new(big.Int).SetBytes(leaf.Data[at : at+32])
		if !n.IsUint64() || n.Uint64() > uint64(len(leaf.Data)) {
			return 0, errors.New("connector signature offset is out of bounds")
		}
		return int(n.Uint64()), nil
	}
	tuple, err := word(4)
	if err != nil || tuple != 128 {
		return nil, errors.New("connector signature tuple offset is malformed")
	}
	first, err := word(4 + tuple + 6*32)
	if err != nil || first != 256 {
		return nil, errors.New("connector signer signature offset is malformed")
	}
	second, err := word(36)
	if err != nil {
		return nil, err
	}
	first += 4 + tuple
	second += 4
	capacity := func(at int, signature []byte) (int, error) {
		if at < 4 || at+32 > len(leaf.Data) {
			return 0, errors.New("connector signature length is out of bounds")
		}
		mask := new(big.Int).SetBytes(leaf.DataMask[at : at+32])
		if !mask.IsUint64() || mask.Uint64() == 0 || mask.Uint64() > 65535 {
			return 0, errors.New("connector signature length mask is invalid")
		}
		maximum := int(mask.Uint64())
		if maximum&(maximum+1) != 0 || len(signature) > maximum || at+32+maximum > len(leaf.Data) {
			return 0, errors.New("connector signature exceeds committed capacity")
		}
		return maximum, nil
	}
	firstCapacity, err := capacity(first, signed.SignerSignature)
	if err != nil {
		return nil, err
	}
	if len(protocol) != 65 || second+32+96 != len(leaf.Data) ||
		new(big.Int).SetBytes(leaf.Data[second:second+32]).Cmp(big.NewInt(65)) != 0 ||
		new(big.Int).SetBytes(leaf.DataMask[second:second+32]).Sign() != 0 {
		return nil, errors.New("connector protocol signature must have a fixed 65-byte layout")
	}
	firstLength := len(signed.SignerSignature)
	signed.SignerSignature = common.RightPadBytes(signed.SignerSignature, firstCapacity)
	fitted, err := connectorBinding.TryPackSwap(signed, protocol, recipient, amount)
	if err != nil {
		return nil, errors.Errorf("encode bounded connector swap: %w", err)
	}
	if len(fitted) != len(leaf.Data) || second != first+32+(firstCapacity+31)/32*32 {
		return nil, errors.New("connector signature buffers do not match committed offsets")
	}
	new(big.Int).SetUint64(uint64(firstLength)).FillBytes(fitted[first : first+32])
	return fitted, nil
}
