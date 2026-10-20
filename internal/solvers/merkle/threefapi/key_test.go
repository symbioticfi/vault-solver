package threefapi

import (
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

func TestKeyGenerationGolden(t *testing.T) {
	s, err := signer.NewFromHexKey("ac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80")
	if err != nil {
		t.Fatal(err)
	}
	digest, err := APIKeyDigest(s.Address(), big.NewInt(4_102_444_800))
	if err != nil {
		t.Fatal(err)
	}
	sig, err := s.SignHash(digest)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "0x804755534b98cba2ac903a07923c4e7e80a02d5ffdc699280dd799faec905af0" +
		"4de6933caf26995e9e41eb4f69d8ac2f600d2c08c610a9548a9b1072a3c1757c1c"
	if hexutil.Encode(sig) != expected {
		t.Fatal("key authorization domain or message changed")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/facilitator/generate-key" || r.Header.Get("x-api-key") != "" {
			t.Error("incorrect key generation request")
		}
		var dto threef.GenerateFacilitatorApiKeyDto
		if decodeErr := json.NewDecoder(r.Body).Decode(&dto); decodeErr != nil {
			t.Fatal(decodeErr)
		}
		deadline, valid := new(big.Int).SetString(dto.Deadline, 10)
		if !valid || !deadline.IsInt64() || deadline.Int64() < time.Now().Unix() || deadline.Int64() > time.Now().Add(6*time.Minute).Unix() || dto.ChainId != 1 || dto.Facilitator != lower(s.Address()) {
			t.Fatal("incorrect key authorization")
		}
		wireDigest, digestErr := APIKeyDigest(s.Address(), deadline)
		wireSignature, sigErr := hexutil.Decode(dto.Signature)
		if digestErr != nil || sigErr != nil || len(wireSignature) != 65 {
			t.Fatal("invalid authorization signature")
		}
		wireSignature[64] -= 27
		pub, recoverErr := crypto.SigToPub(wireDigest.Bytes(), wireSignature)
		if recoverErr != nil || crypto.PubkeyToAddress(*pub) != s.Address() {
			t.Fatal("incorrect signer")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiKey":"fixture-generated-value"}`))
	}))
	t.Cleanup(server.Close)
	key, err := GenerateKey(t.Context(), server.URL, s)
	if err != nil || key != "fixture-generated-value" {
		t.Fatalf("generate key: %v", err)
	}
}
