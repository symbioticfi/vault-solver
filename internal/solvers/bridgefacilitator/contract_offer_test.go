package bridgefacilitator

import (
	stderrors "errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

const contractPrivateKey = "ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"

type contractRecordingSigner struct {
	signer.Signer

	hashes []common.Hash
	failAt map[int]error
}

func (s *contractRecordingSigner) SignHash(hash common.Hash) ([]byte, error) {
	s.hashes = append(s.hashes, hash)
	if err := s.failAt[len(s.hashes)]; err != nil {
		return nil, err
	}
	return s.Signer.SignHash(hash)
}

// Test3FR1YieldContract is the immutable 3F-R1 solver-side yield backstop baseline.
func Test3FR1YieldContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                               string
		expectedReturn, principal, minimum *big.Int
		minimumYieldPpm                    *big.Int
		maxRateBps                         float64
		want                               string
	}{
		{name: "nil principal", expectedReturn: big.NewInt(1), want: "invalid offer amounts (must be positive): principal=<nil> expectedReturn=1"},
		{name: "zero principal", expectedReturn: big.NewInt(1), principal: big.NewInt(0), want: "invalid offer amounts (must be positive): principal=0 expectedReturn=1"},
		{name: "negative principal", expectedReturn: big.NewInt(1), principal: big.NewInt(-1), want: "invalid offer amounts (must be positive): principal=-1 expectedReturn=1"},
		{name: "nil expected return", principal: big.NewInt(100), want: "invalid offer amounts (must be positive): principal=100 expectedReturn=<nil>"},
		{name: "zero expected return", expectedReturn: big.NewInt(0), principal: big.NewInt(100), want: "invalid offer amounts (must be positive): principal=100 expectedReturn=0"},
		{name: "negative expected return", expectedReturn: big.NewInt(-1), principal: big.NewInt(100), want: "invalid offer amounts (must be positive): principal=100 expectedReturn=-1"},
		{name: "below minimum principal", expectedReturn: big.NewInt(2), principal: big.NewInt(99), minimum: big.NewInt(100), want: "principal 99 below minAssetsPerRequest 100"},
		{name: "below yield floor", expectedReturn: big.NewInt(189), principal: big.NewInt(1_000_000), minimumYieldPpm: big.NewInt(190), want: "yield below minYieldPerRequest floor 190 ppm"},
		{name: "over auction cap", expectedReturn: big.NewInt(21), principal: big.NewInt(1000), maxRateBps: 200, want: "yield above auction maxRate 200 bps"},
		{name: "floor exact but partial unsafe", expectedReturn: big.NewInt(1), principal: big.NewInt(1000), minimum: big.NewInt(100), minimumYieldPpm: big.NewInt(190), maxRateBps: 250, want: "yield is unsafe for partial consumption: margin 0 below required 10"},
		{name: "positive margin but partial unsafe", expectedReturn: big.NewInt(2), principal: big.NewInt(1000), minimum: big.NewInt(100), minimumYieldPpm: big.NewInt(190), maxRateBps: 250, want: "yield is unsafe for partial consumption: margin 1 below required 10"},
		{name: "zero minimum requires compatibility margin", expectedReturn: big.NewInt(2), principal: big.NewInt(1000), minimum: big.NewInt(0), minimumYieldPpm: big.NewInt(190), maxRateBps: 250, want: "yield is unsafe for partial consumption: margin 1 below required 2"},
		{name: "zero minimum compatibility margin is safe", expectedReturn: big.NewInt(3), principal: big.NewInt(1000), minimum: big.NewInt(0), minimumYieldPpm: big.NewInt(190), maxRateBps: 250},
		{name: "partial safe", expectedReturn: big.NewInt(11), principal: big.NewInt(1000), minimum: big.NewInt(100), minimumYieldPpm: big.NewInt(190), maxRateBps: 250},
		{name: "full principal minimum needs no margin", expectedReturn: big.NewInt(1), principal: big.NewInt(1000), minimum: big.NewInt(1000), minimumYieldPpm: big.NewInt(190), maxRateBps: 250},
		{name: "nonpositive floor and cap disable rate checks", expectedReturn: big.NewInt(1), principal: big.NewInt(1000), minimumYieldPpm: big.NewInt(-1), maxRateBps: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateYield(test.expectedReturn, test.principal, test.minimum, test.minimumYieldPpm, test.maxRateBps)
			if test.want == "" {
				if err != nil {
					t.Fatalf("ValidateYield: %v", err)
				}
				return
			}
			if err == nil || err.Error() != test.want {
				t.Fatalf("ValidateYield error = %v, want %q", err, test.want)
			}
		})
	}
}

// Test3FR1SignedOfferContract freezes nullable-domain handling, version fallback, expiration,
// nonce consumption on signer failure, the selected digest, and the resulting complete DTO.
func Test3FR1SignedOfferContract(t *testing.T) { //nolint:cyclop,maintidx // One immutable table spans domain, nonce, signature, DTO, and expiration behavior.
	t.Parallel()

	baseSigner, err := signer.NewFromHexKey(contractPrivateKey)
	if err != nil {
		t.Fatalf("NewFromHexKey: %v", err)
	}
	maker := common.HexToAddress("0x1111111111111111111111111111111111111111")
	strategyRequest := common.HexToAddress("0x2222222222222222222222222222222222222222")
	execution := OfferExecution{
		AuctionID: 77, Request: strategyRequest, Maker: maker,
		Principal: big.NewInt(1_000_000), ExpectedReturn: big.NewInt(20_000),
	}
	valid := func() threef.AuctionDto {
		dto := contractAuctionDTO(77, "0x9999999999999999999999999999999999999999", "0x3333333333333333333333333333333333333333")
		dto.SetSolveStartTime("2100-01-01T00:00:00Z")
		return dto
	}

	tests := []struct {
		name   string
		mutate func(*threef.AuctionDto)
		want   string
	}{
		{name: "missing domain", mutate: func(d *threef.AuctionDto) { d.UnsetEip712Domain() }, want: "auction 77: missing EIP-712 domain"},
		{name: "null domain", mutate: func(d *threef.AuctionDto) { d.SetEip712DomainNil() }, want: "auction 77: missing EIP-712 domain"},
		{name: "missing name", mutate: func(d *threef.AuctionDto) {
			domain := d.GetEip712Domain()
			domain.Name.Unset()
			d.SetEip712Domain(domain)
		}, want: "auction 77: missing EIP-712 domain name"},
		{name: "null name", mutate: func(d *threef.AuctionDto) {
			domain := d.GetEip712Domain()
			domain.Name.Set(nil)
			d.SetEip712Domain(domain)
		}, want: "auction 77: missing EIP-712 domain name"},
		{name: "missing chain", mutate: func(d *threef.AuctionDto) {
			domain := d.GetEip712Domain()
			domain.ChainId.Unset()
			d.SetEip712Domain(domain)
		}, want: "auction 77: missing EIP-712 domain chainId"},
		{name: "null chain", mutate: func(d *threef.AuctionDto) {
			domain := d.GetEip712Domain()
			domain.ChainId.Set(nil)
			d.SetEip712Domain(domain)
		}, want: "auction 77: missing EIP-712 domain chainId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			dto := valid()
			test.mutate(&dto)
			s := &Solver{cfg: &Config{OfferExpiryBuffer: 2 * time.Hour}, signer: baseSigner}
			_, err := s.buildSignedOffer(auctionView{dto}, execution)
			if err == nil || err.Error() != test.want {
				t.Fatalf("buildSignedOffer error = %v, want %q", err, test.want)
			}
			if s.nonceSeq != 0 {
				t.Fatalf("domain rejection consumed nonce %d, want 0", s.nonceSeq)
			}
		})
	}

	const (
		wantFallbackDigest    = "0x7fe195c5613f26a03be7e2c7ecd844f3f58fc795c1e27dc9af5d7c0970179094"
		wantFallbackSignature = "0xebd586430ad85463f38cf5655a8a5c39073445713d90767b62d9c1c1bf3eeee44ac996a4053b8a266f883b34997a14a49976c99fa73deba1ef8cf19af0be4c4b1b"
	)
	versionCases := []struct {
		name   string
		mutate func(*threef.AuctionEip712DomainDto)
	}{
		{name: "version absent", mutate: func(d *threef.AuctionEip712DomainDto) { d.Version.Unset() }},
		{name: "version null", mutate: func(d *threef.AuctionEip712DomainDto) { d.Version.Set(nil) }},
		{name: "version empty", mutate: func(d *threef.AuctionEip712DomainDto) { d.SetVersion("") }},
	}
	for _, test := range versionCases {
		dto := valid()
		domain := dto.GetEip712Domain()
		test.mutate(&domain)
		dto.SetEip712Domain(domain)
		recorder := &contractRecordingSigner{Signer: baseSigner}
		s := &Solver{cfg: &Config{OfferExpiryBuffer: 2 * time.Hour}, signer: recorder, nonceSeq: 40}
		got, err := s.buildSignedOffer(auctionView{dto}, execution)
		if err != nil {
			t.Fatalf("%s: buildSignedOffer: %v", test.name, err)
		}
		if digest, signature := recorder.hashes[0].Hex(), got.GetSignature(); digest != wantFallbackDigest || signature != wantFallbackSignature {
			t.Fatalf("%s fallback changed: digest=%s signature=%s", test.name, digest, signature)
		}
	}

	customDTO := valid()
	recorder := &contractRecordingSigner{Signer: baseSigner, failAt: map[int]error{1: stderrors.New("injected signing failure")}}
	s := &Solver{cfg: &Config{OfferExpiryBuffer: 2 * time.Hour}, signer: recorder, nonceSeq: 500}
	if _, err := s.buildSignedOffer(auctionView{customDTO}, execution); err == nil || err.Error() != "sign offer: injected signing failure" {
		t.Fatalf("first build error = %v", err)
	}
	got, err := s.buildSignedOffer(auctionView{customDTO}, execution)
	if err != nil {
		t.Fatalf("second buildSignedOffer: %v", err)
	}
	if s.nonceSeq != 502 || got.Nonce != "502" {
		t.Fatalf("nonce sequence=%d dto.nonce=%q, want failed 501 consumed and success 502", s.nonceSeq, got.Nonce)
	}
	const (
		wantFailedDigest = "0x94d80923ace9b7e885b13fb2f36ef92c04d2e059d9e1f217e454276f0f823735"
		wantDigest       = "0xd2b2cb72df430698a1baeb23b2f35d9f1c1c7606da167646b3720faa70e30c73"
		wantSignature    = "0x5ee271d2c7b4c9565fae722ee2376172894c143075981e939d40bf854ecc34515bf220af8ca52083c1003540024590ce09bd40fa25c56f82f472c93479cfd9b81c"
	)
	if recorder.hashes[0].Hex() != wantFailedDigest || recorder.hashes[1].Hex() != wantDigest {
		t.Fatalf("digest changed: failed=%s success=%s", recorder.hashes[0].Hex(), recorder.hashes[1].Hex())
	}
	if got.AuctionId != 77 || got.Maker != "0x1111111111111111111111111111111111111111" || got.Amount != "1000000" ||
		got.ExpectedReturn != "20000" || got.Nonce != "502" || got.Expiration != "4102452000" || !got.UseCallback ||
		got.GetChainId() != 11155111 || got.GetSignature() != wantSignature {
		t.Fatalf("signed DTO changed: %+v", got)
	}
	if len(got.GetSignature()) != 132 {
		t.Fatalf("signature length = %d, want 132", len(got.GetSignature()))
	}
	sig := common.FromHex(got.GetSignature())
	sig[64] -= 27
	pub, err := crypto.SigToPub(recorder.hashes[1].Bytes(), sig)
	if err != nil {
		t.Fatalf("recover signer: %v", err)
	}
	if recovered := crypto.PubkeyToAddress(*pub); recovered != baseSigner.Address() || recovered.Hex() != "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266" {
		t.Fatalf("recovered signer = %s, want deterministic key address", recovered.Hex())
	}

	customDomain := customDTO.GetEip712Domain()
	customDomain.SetVersion("custom-v2")
	customDTO.SetEip712Domain(customDomain)
	customRecorder := &contractRecordingSigner{Signer: baseSigner}
	customSolver := &Solver{cfg: &Config{OfferExpiryBuffer: 2 * time.Hour}, signer: customRecorder, nonceSeq: 501}
	custom, err := customSolver.buildSignedOffer(auctionView{customDTO}, execution)
	if err != nil {
		t.Fatalf("custom version: %v", err)
	}
	const (
		wantCustomDigest    = "0xdf2d84614099a0ff84f6e7c4e2c504bc3aee20b7148a9cd775001b2906fe9db8"
		wantCustomSignature = "0x771fd4e0fedb3af95261473c432d72a6b59e836f7e5571888c3d3a76f4a3173248258bf6707d2d98472a9115d30da132bd85df0bbcf45eb22595833f78a7e43b1c"
	)
	if digest, signature := customRecorder.hashes[0].Hex(), custom.GetSignature(); digest != wantCustomDigest || signature != wantCustomSignature {
		t.Fatalf("custom domain version changed: digest=%s signature=%s", digest, signature)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	expirationCases := []struct {
		name, solveStart string
		set              bool
		want             int64
	}{
		{name: "missing", want: 1_700_007_200},
		{name: "explicit null", set: true, want: 1_700_007_200},
		{name: "empty", solveStart: "", set: true, want: 1_700_007_200},
		{name: "invalid", solveStart: "not-a-time", set: true, want: 1_700_007_200},
		{name: "parseable", solveStart: "2100-01-01T00:00:00Z", set: true, want: 4_102_452_000},
	}
	for _, test := range expirationCases {
		dto := valid()
		dto.SolveStartTime.Unset()
		if test.set {
			if test.name == "explicit null" {
				dto.SolveStartTime.Set(nil)
			} else {
				dto.SetSolveStartTime(test.solveStart)
			}
		}
		if expiration := offerExpiration(auctionView{dto}, 2*time.Hour, now).Int64(); expiration != test.want {
			t.Fatalf("%s expiration = %d, want %d", test.name, expiration, test.want)
		}
	}
}
