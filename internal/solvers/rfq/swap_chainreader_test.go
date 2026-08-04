package rfq

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/liquidlane/adapter"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func TestSwapOnchainReaderValidateRouterRequiresDeployedCode(t *testing.T) {
	router := common.HexToAddress(testRouter)
	reader := &swapOnchainReader{chain: &fakeSwapChain{code: []byte{0x60, 0x00}}, chainID: 1}
	if err := reader.validateRouter(t.Context(), router); err != nil {
		t.Fatalf("validateRouter: %v", err)
	}
	reader.chain = &fakeSwapChain{}
	if err := reader.validateRouter(t.Context(), router); err == nil {
		t.Fatal("empty Router bytecode was accepted")
	}
	reader.chain = &fakeSwapChain{codeErr: errors.New("rpc unavailable")}
	if err := reader.validateRouter(t.Context(), router); err == nil {
		t.Fatal("Router code dependency failure was accepted")
	}
}

func TestSwapOnchainReaderValidateAdaptersAcceptsAuthorizedDomain(t *testing.T) {
	adapterAddress := common.HexToAddress(testAdapter)
	signer := common.HexToAddress(testSwapper)
	chainBackend := &fakeSwapChain{results: []chain.CallResult{{
		Success: true, ReturnData: packSwapDomainOutput(t, 0x0f, "LiquidLaneAdapter", "1", 1, adapterAddress, [32]byte{}, nil),
	}}}
	ll := &fakeSwapLiquidLaneReader{auth: []liquidlane.Auth{{Adapter: adapterAddress, Authorized: true}}}
	reader := &swapOnchainReader{chain: chainBackend, ll: ll, chainID: 1}

	domains, err := reader.validateAdapters(t.Context(), []common.Address{adapterAddress, adapterAddress}, signer)
	if err != nil {
		t.Fatalf("validateAdapters: %v", err)
	}
	if len(domains) != 1 || domains[adapterAddress].Name != "LiquidLaneAdapter" ||
		domains[adapterAddress].VerifyingContract != adapterAddress {
		t.Fatalf("domains = %+v", domains)
	}
	if len(chainBackend.calls) != 1 || len(chainBackend.calls[0]) != 1 {
		t.Fatalf("domain reads were not deduplicated: %+v", chainBackend.calls)
	}
}

func TestSwapOnchainReaderValidateAdaptersRejectsUnauthorizedSigner(t *testing.T) {
	adapterAddress := common.HexToAddress(testAdapter)
	reader := &swapOnchainReader{
		chain: &fakeSwapChain{}, ll: &fakeSwapLiquidLaneReader{
			auth: []liquidlane.Auth{{Adapter: adapterAddress, Authorized: false}},
		}, chainID: 1,
	}
	if _, err := reader.validateAdapters(t.Context(), []common.Address{adapterAddress}, common.HexToAddress(testSwapper)); err == nil {
		t.Fatal("unauthorized signer was accepted")
	}
}

func TestSwapOnchainReaderValidateAdaptersRejectsInvalidDomainShapes(t *testing.T) {
	adapterAddress := common.HexToAddress(testAdapter)
	signer := common.HexToAddress(testSwapper)
	cases := map[string][]byte{
		"fields":   packSwapDomainOutput(t, 0x1f, "LiquidLaneAdapter", "1", 1, adapterAddress, [32]byte{}, nil),
		"name":     packSwapDomainOutput(t, 0x0f, "", "1", 1, adapterAddress, [32]byte{}, nil),
		"chain":    packSwapDomainOutput(t, 0x0f, "LiquidLaneAdapter", "1", 2, adapterAddress, [32]byte{}, nil),
		"verifier": packSwapDomainOutput(t, 0x0f, "LiquidLaneAdapter", "1", 1, common.HexToAddress(testRouter), [32]byte{}, nil),
		"salt": func() []byte {
			var salt [32]byte
			salt[0] = 1
			return packSwapDomainOutput(t, 0x0f, "LiquidLaneAdapter", "1", 1, adapterAddress, salt, nil)
		}(),
		"extensions": packSwapDomainOutput(t, 0x0f, "LiquidLaneAdapter", "1", 1, adapterAddress, [32]byte{}, []*big.Int{big.NewInt(1)}),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			reader := &swapOnchainReader{
				chain:   &fakeSwapChain{results: []chain.CallResult{{Success: true, ReturnData: data}}},
				ll:      &fakeSwapLiquidLaneReader{auth: []liquidlane.Auth{{Adapter: adapterAddress, Authorized: true}}},
				chainID: 1,
			}
			if _, err := reader.validateAdapters(t.Context(), []common.Address{adapterAddress}, signer); err == nil {
				t.Fatal("invalid EIP-712 domain was accepted")
			}
		})
	}
}

func TestSwapOnchainReaderReadFillQuoteRequiresExactRouteAndAmount(t *testing.T) {
	route := liquidlane.NewRoute(
		1, common.HexToAddress(testAdapter), common.HexToAddress(testVault), common.HexToAddress(testTokenIn),
		common.HexToAddress(testTokenOut), 18, 6,
	)
	quote := liquidlane.FillQuote{Inventory: liquidlane.DirectInventory(route, big.NewInt(20), big.NewInt(1)), AmountIn: big.NewInt(10), MaxAmountOut: big.NewInt(19)}
	reader := &swapOnchainReader{ll: &fakeSwapLiquidLaneReader{quotes: []liquidlane.FillQuote{quote}}, chainID: 1}
	got, err := reader.readFillQuote(t.Context(), route, big.NewInt(10))
	if err != nil || got.AmountIn.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("readFillQuote = %+v, err %v", got, err)
	}
	reader.ll = &fakeSwapLiquidLaneReader{quotes: []liquidlane.FillQuote{quote, quote}}
	if _, err := reader.readFillQuote(t.Context(), route, big.NewInt(10)); err == nil {
		t.Fatal("multiple exact-route quotes were accepted")
	}
	wrong := quote
	wrong.Route.Adapter = common.HexToAddress(testRouter)
	reader.ll = &fakeSwapLiquidLaneReader{quotes: []liquidlane.FillQuote{wrong}}
	if _, err := reader.readFillQuote(t.Context(), route, big.NewInt(10)); err == nil {
		t.Fatal("changed route identity was accepted")
	}
}

func TestSwapOnchainReaderReadUsedNoncesFailsClosed(t *testing.T) {
	check := swapNonceCheck{Adapter: common.HexToAddress(testAdapter), TokenIn: common.HexToAddress(testTokenIn), Nonce: big.NewInt(9)}
	reader := &swapOnchainReader{
		chain:   &fakeSwapChain{results: []chain.CallResult{{Success: true, ReturnData: packUsedNonceOutput(t, true)}}},
		chainID: 1,
	}
	used, err := reader.readUsedNonces(t.Context(), []swapNonceCheck{check})
	if err != nil || len(used) != 1 || !used[0] {
		t.Fatalf("readUsedNonces = %v, err %v", used, err)
	}
	reader.chain = &fakeSwapChain{results: []chain.CallResult{{Success: false}}}
	if _, err := reader.readUsedNonces(t.Context(), []swapNonceCheck{check}); err == nil {
		t.Fatal("failed nonce read was treated as unused")
	}
	reader.chain = &fakeSwapChain{multicallErr: errors.New("rpc unavailable")}
	if _, err := reader.readUsedNonces(t.Context(), []swapNonceCheck{check}); err == nil {
		t.Fatal("nonce transport failure was treated as unused")
	}
}

type fakeSwapChain struct {
	code         []byte
	codeErr      error
	results      []chain.CallResult
	multicallErr error
	calls        [][]chain.Call
}

func (f *fakeSwapChain) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return append([]byte(nil), f.code...), f.codeErr
}

func (f *fakeSwapChain) Multicall(_ context.Context, calls []chain.Call) ([]chain.CallResult, error) {
	f.calls = append(f.calls, append([]chain.Call(nil), calls...))
	return append([]chain.CallResult(nil), f.results...), f.multicallErr
}

type fakeSwapLiquidLaneReader struct {
	auth      []liquidlane.Auth
	authErr   error
	quotes    []liquidlane.FillQuote
	quotesErr error
}

func (f *fakeSwapLiquidLaneReader) ReadAuth(context.Context, []common.Address, common.Address) ([]liquidlane.Auth, error) {
	return append([]liquidlane.Auth(nil), f.auth...), f.authErr
}

func (f *fakeSwapLiquidLaneReader) ReadFillQuotes(context.Context, []liquidlane.Route, common.Address, *big.Int) ([]liquidlane.FillQuote, error) {
	return append([]liquidlane.FillQuote(nil), f.quotes...), f.quotesErr
}

func packSwapDomainOutput(
	t *testing.T,
	fields byte,
	name string,
	version string,
	chainID int64,
	verifyingContract common.Address,
	salt [32]byte,
	extensions []*big.Int,
) []byte {
	t.Helper()
	parsed, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	data, err := parsed.Methods["eip712Domain"].Outputs.Pack(
		[1]byte{fields}, name, version, big.NewInt(chainID), verifyingContract, salt, extensions,
	)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func packUsedNonceOutput(t *testing.T, used bool) []byte {
	t.Helper()
	parsed, err := adapter.LiquidLaneAdapterMetaData.ParseABI()
	if err != nil {
		t.Fatal(err)
	}
	data, err := parsed.Methods["isUsedNonce"].Outputs.Pack(used)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
