// Package connector prepares Liquid Lane connector swaps funded through Merkle recipes.
// It owns recipe route discovery, signed-discount framing, recipe assembly and the
// capacity ledger; each settlement protocol wraps the prepared recipe call in its own
// outer transaction.
package connector

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	sdkparse "github.com/symbioticfi/merkle-executor/sdk/parse"
	binding "github.com/symbioticfi/vault-solver/api/bindings/liquidlane/connector"
	"github.com/symbioticfi/vault-solver/internal/liquidlane/discounts"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

var connectorBinding = binding.NewLiquidLaneConnector()

// Terms are the unchanged upstream signed discount terms for one connector swap.
type Terms struct {
	Swap       binding.ILiquidLaneConnectorDiscountSwap
	Signature  []byte
	ValidUntil time.Time
}

// ParseDiscount frames resolved terms for the selected connector route.
// Signed terms are upstream-owned; only ABI framing and selected route identity are checked here.
func ParseDiscount(r *discounts.Resolved, id common.Hash, venue, token common.Address) (Terms, error) {
	var out binding.ILiquidLaneConnectorDiscountSwap
	if r == nil {
		return Terms{}, errors.New("connector discount is nil")
	}
	if r.DiscountPrecision != PrecisionText {
		return Terms{}, errors.Errorf("connector discount precision: expected %s, got %q", PrecisionText, r.DiscountPrecision)
	}
	rawID, err := hexutil.Decode(r.DiscountID)
	if err != nil || len(rawID) != 32 || common.BytesToHash(rawID) != id {
		return Terms{}, errors.New("connector discount id does not match selected route")
	}
	a, err := parse.NonZeroAddress(r.Discount.Adapter, "connector adapter")
	if err != nil || a != venue {
		return Terms{}, errors.New("connector discount route mismatch")
	}
	out.Discount.TokenToRedeem, err = parse.NonZeroAddress(r.Discount.TokenToRedeem, "tokenToRedeem")
	if err != nil || out.Discount.TokenToRedeem != token {
		return Terms{}, errors.New("connector discount route mismatch")
	}
	out.Discount.Signer, err = parse.NonZeroAddress(r.Discount.Signer, "signer")
	if err != nil {
		return Terms{}, err
	}
	out.Discount.Protocol, err = parse.NonZeroAddress(r.Discount.Protocol, "protocol")
	if err != nil {
		return Terms{}, err
	}
	out.Discount.Discount, err = sdkparse.Uint256(r.Discount.Discount)
	if err != nil {
		return Terms{}, errors.Errorf("discount: %w", err)
	}
	out.Discount.Nonce, err = sdkparse.Uint256(r.Discount.Nonce)
	if err != nil {
		return Terms{}, errors.Errorf("nonce: %w", err)
	}
	if r.Discount.Deadline < 0 || r.Discount.Deadline >= 1<<48 || r.ProtocolDeadline < 0 || r.ProtocolDeadline >= 1<<48 {
		return Terms{}, errors.New("connector discount deadline exceeds uint48")
	}
	out.Discount.Deadline = big.NewInt(r.Discount.Deadline)
	out.ProtocolDeadline = big.NewInt(r.ProtocolDeadline)
	out.SignerSignature, err = hexutil.Decode(r.SignerSignature)
	if err != nil {
		return Terms{}, errors.Errorf("signerSignature: %w", err)
	}
	sig, err := hexutil.Decode(r.ProtocolSignature)
	if err != nil {
		return Terms{}, errors.Errorf("protocolSignature: %w", err)
	}
	until := min(r.Discount.Deadline, r.ProtocolDeadline)
	return Terms{out, sig, time.Unix(until, 0)}, nil
}
