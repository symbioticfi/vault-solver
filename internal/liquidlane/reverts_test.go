package liquidlane

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/bindings/erc20"
	"github.com/symbioticfi/vault-solver/api/bindings/rfq/reactor"
)

func TestRevertDecoderNamesCoreAndIntegrationErrors(t *testing.T) {
	t.Parallel()
	decoder := RevertDecoder(&reactor.ReactorMetaData)
	for signature, want := range map[string]string{
		"InvalidDiscount()":          "InvalidDiscount()",          // LiquidLaneAdapter
		"InvalidProtocolSignature()": "InvalidProtocolSignature()", // Reactor, passed by the integration
	} {
		if got := decoder.Describe(crypto.Keccak256([]byte(signature))[:4]); got != want {
			t.Fatalf("%s: Describe = %q, want %q", signature, got, want)
		}
	}
}

func TestRevertDecoderNamesERC20Errors(t *testing.T) {
	t.Parallel()
	spender := common.HexToAddress("0x00000000000000000000000000000000000000aa")
	data, err := erc20.NewERC20().GetABI().Errors["ERC20InsufficientAllowance"].Inputs.Pack(spender, big.NewInt(0), big.NewInt(5))
	if err != nil {
		t.Fatal(err)
	}
	payload := append(crypto.Keccak256([]byte("ERC20InsufficientAllowance(address,uint256,uint256)"))[:4], data...)
	want := "ERC20InsufficientAllowance(spender=" + spender.Hex() + ", allowance=0, needed=5)"
	if got := RevertDecoder().Describe(payload); got != want {
		t.Fatalf("Describe = %q, want %q", got, want)
	}
}
