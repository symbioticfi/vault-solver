package uniswapx

import (
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/symbioticfi/vault-solver/internal/tokenpolicy"
)

const goldenV2OrderHash = "0x4bd12c75e25c9601d854988baadbdd0ad1a147b3cb55b0899e143f8db9f3be6e"

func TestV2OrderHashMatchesApitypesAndGolden(t *testing.T) {
	order := v2Order{
		Info: v2OrderInfo{
			Reactor: common.HexToAddress("0x1111111111111111111111111111111111111111"),
			Swapper: common.HexToAddress("0x2222222222222222222222222222222222222222"),
			Nonce:   big.NewInt(7), Deadline: big.NewInt(1_900_000_000),
			AdditionalValidationContract: common.HexToAddress("0x3333333333333333333333333333333333333333"),
			AdditionalValidationData:     hexutil.MustDecode("0x123456"),
		},
		Cosigner: common.HexToAddress("0x4444444444444444444444444444444444444444"),
		BaseInput: v2Input{
			Token:       common.HexToAddress("0x5555555555555555555555555555555555555555"),
			StartAmount: big.NewInt(1_000_000), EndAmount: big.NewInt(1_100_000),
		},
		BaseOutputs: []v2Output{{
			Token:       common.HexToAddress("0x6666666666666666666666666666666666666666"),
			StartAmount: big.NewInt(2_000_000), EndAmount: big.NewInt(1_800_000),
			Recipient: common.HexToAddress("0x7777777777777777777777777777777777777777"),
		}},
	}

	got, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hex() != goldenV2OrderHash {
		t.Fatalf("V2 order hash = %s, want golden %s", got.Hex(), goldenV2OrderHash)
	}

	typed := apitypes.TypedData{
		Types: apitypes.Types{
			"EIP712Domain": {{Name: "name", Type: "string"}},
			"V2DutchOrder": {
				{Name: "info", Type: "OrderInfo"}, {Name: "cosigner", Type: "address"},
				{Name: "baseInputToken", Type: "address"}, {Name: "baseInputStartAmount", Type: "uint256"},
				{Name: "baseInputEndAmount", Type: "uint256"}, {Name: "baseOutputs", Type: "DutchOutput[]"},
			},
			"OrderInfo": {
				{Name: "reactor", Type: "address"}, {Name: "swapper", Type: "address"},
				{Name: "nonce", Type: "uint256"}, {Name: "deadline", Type: "uint256"},
				{Name: "additionalValidationContract", Type: "address"},
				{Name: "additionalValidationData", Type: "bytes"},
			},
			"DutchOutput": {
				{Name: "token", Type: "address"}, {Name: "startAmount", Type: "uint256"},
				{Name: "endAmount", Type: "uint256"}, {Name: "recipient", Type: "address"},
			},
		},
		PrimaryType: "V2DutchOrder",
		Domain:      apitypes.TypedDataDomain{Name: "unused"},
		Message: apitypes.TypedDataMessage{
			"info": map[string]any{
				"reactor": order.Info.Reactor.Hex(), "swapper": order.Info.Swapper.Hex(),
				"nonce": order.Info.Nonce.String(), "deadline": order.Info.Deadline.String(),
				"additionalValidationContract": order.Info.AdditionalValidationContract.Hex(),
				"additionalValidationData":     hexutil.Encode(order.Info.AdditionalValidationData),
			},
			"cosigner": order.Cosigner.Hex(), "baseInputToken": order.BaseInput.Token.Hex(),
			"baseInputStartAmount": order.BaseInput.StartAmount.String(),
			"baseInputEndAmount":   order.BaseInput.EndAmount.String(),
			"baseOutputs": []any{map[string]any{
				"token":       order.BaseOutputs[0].Token.Hex(),
				"startAmount": order.BaseOutputs[0].StartAmount.String(),
				"endAmount":   order.BaseOutputs[0].EndAmount.String(),
				"recipient":   order.BaseOutputs[0].Recipient.Hex(),
			}},
		},
	}
	want, err := typed.HashStruct(typed.PrimaryType, typed.Message)
	if err != nil {
		t.Fatalf("hash V2 order with apitypes: %v", err)
	}
	if got != common.BytesToHash(want) {
		t.Fatalf("V2 hash mismatch:\n manual   %s\n apitypes %s", got.Hex(), common.BytesToHash(want).Hex())
	}
}

func TestParseAndResolveOrder(t *testing.T) {
	reactor := common.HexToAddress("0x1111111111111111111111111111111111111111")
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	cosignerKey, err := crypto.HexToECDSA("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	cosigner := crypto.PubkeyToAddress(cosignerKey.PublicKey)
	tokenIn := common.HexToAddress("0x4444444444444444444444444444444444444444")
	tokenOut := common.HexToAddress("0x5555555555555555555555555555555555555555")
	recipient := common.HexToAddress("0x6666666666666666666666666666666666666666")
	policy, err := tokenpolicy.New(tokenpolicy.All, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{Reactor: reactor, Executor: executor, TokenPolicy: policy}
	order := v2Order{
		Info: v2OrderInfo{
			Reactor: reactor, Swapper: recipient, Nonce: big.NewInt(1), Deadline: big.NewInt(1_200),
			AdditionalValidationData: []byte{},
		},
		Cosigner:    cosigner,
		BaseInput:   v2Input{Token: tokenIn, StartAmount: big.NewInt(100), EndAmount: big.NewInt(100)},
		BaseOutputs: []v2Output{{Token: tokenOut, StartAmount: big.NewInt(220), EndAmount: big.NewInt(200), Recipient: recipient}},
		CosignerData: v2CosignerData{
			DecayStartTime: big.NewInt(1_000), DecayEndTime: big.NewInt(1_100), ExclusiveFiller: executor,
			ExclusivityOverrideBps: big.NewInt(0), InputOverride: big.NewInt(0), OutputOverrides: []*big.Int{big.NewInt(0)},
		},
		Cosignature: make([]byte, 65),
	}
	hash, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	cosignerData, err := v2CosignerDataArguments.Pack(order.CosignerData)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature, err = crypto.Sign(crypto.Keccak256(hash.Bytes(), cosignerData), cosignerKey)
	if err != nil {
		t.Fatal(err)
	}
	order.Cosignature[64] += 27
	encoded, err := v2OrderArguments.Pack(order)
	if err != nil {
		t.Fatalf("pack order: %v", err)
	}
	entry := orderEntry{
		Type: "Dutch_V2", EncodedOrder: hexutil.Encode(encoded), Signature: "0x01", OrderHash: hash.Hex(),
		OrderStatus: "open", ChainID: 1, QuoteID: "quote-1",
		Input:   orderToken{Token: tokenIn.Hex(), StartAmount: "100", EndAmount: "100"},
		Outputs: []orderOutput{{Token: tokenOut.Hex(), StartAmount: "220", EndAmount: "200", Recipient: recipient.Hex()}},
	}
	resolved, err := parseAndResolveOrder(entry, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0))
	if err != nil {
		t.Fatalf("parseAndResolveOrder: %v", err)
	}
	if resolved.AmountIn.Cmp(big.NewInt(100)) != 0 || resolved.AmountOut.Cmp(big.NewInt(210)) != 0 {
		t.Fatalf("resolved amounts = %s/%s, want 100/210", resolved.AmountIn, resolved.AmountOut)
	}

	t.Run("accepts the cosigner authorized by each order", func(t *testing.T) {
		rotatedKey, keyErr := crypto.HexToECDSA(
			"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
		)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		rotated := order
		rotated.Cosigner = crypto.PubkeyToAddress(rotatedKey.PublicKey)
		rotatedHash, hashErr := v2OrderHash(rotated)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		encodedCosignerData, packErr := v2CosignerDataArguments.Pack(rotated.CosignerData)
		if packErr != nil {
			t.Fatal(packErr)
		}
		rotated.Cosignature, packErr = crypto.Sign(
			crypto.Keccak256(rotatedHash.Bytes(), encodedCosignerData),
			rotatedKey,
		)
		if packErr != nil {
			t.Fatal(packErr)
		}
		rotated.Cosignature[64] += 27
		body, packErr := v2OrderArguments.Pack(rotated)
		if packErr != nil {
			t.Fatal(packErr)
		}
		rotatedEntry := entry
		rotatedEntry.EncodedOrder = hexutil.Encode(body)
		rotatedEntry.OrderHash = rotatedHash.Hex()
		if _, parseErr := parseAndResolveOrder(
			rotatedEntry, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0),
		); parseErr != nil {
			t.Fatalf("order-authorized cosigner rejected: %v", parseErr)
		}
	})

	t.Run("public V2 applies active exclusivity override with ceil rounding", func(t *testing.T) {
		publicOrder := order
		publicOrder.CosignerData.ExclusiveFiller = recipient
		publicOrder.CosignerData.ExclusivityOverrideBps = big.NewInt(100)
		publicCosignerData, packErr := v2CosignerDataArguments.Pack(publicOrder.CosignerData)
		if packErr != nil {
			t.Fatal(packErr)
		}
		publicOrder.Cosignature, packErr = crypto.Sign(crypto.Keccak256(hash.Bytes(), publicCosignerData), cosignerKey)
		if packErr != nil {
			t.Fatal(packErr)
		}
		publicOrder.Cosignature[64] += 27
		body, packErr := v2OrderArguments.Pack(publicOrder)
		if packErr != nil {
			t.Fatal(packErr)
		}
		publicEntry := entry
		publicEntry.EncodedOrder = hexutil.Encode(body)
		public, parseErr := parseAndResolveOrder(publicEntry, orderSourcePublicV2, cfg, 1, time.Unix(1_000, 0))
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if public.AmountOut.Cmp(big.NewInt(223)) != 0 {
			t.Fatalf("override amount = %s, want 223", public.AmountOut)
		}

		publicOrder.CosignerData.ExclusivityOverrideBps = big.NewInt(0)
		publicCosignerData, _ = v2CosignerDataArguments.Pack(publicOrder.CosignerData)
		publicOrder.Cosignature, _ = crypto.Sign(crypto.Keccak256(hash.Bytes(), publicCosignerData), cosignerKey)
		publicOrder.Cosignature[64] += 27
		body, _ = v2OrderArguments.Pack(publicOrder)
		publicEntry.EncodedOrder = hexutil.Encode(body)
		if _, parseErr = parseAndResolveOrder(publicEntry, orderSourcePublicV2, cfg, 1, time.Unix(1_000, 0)); parseErr == nil {
			t.Fatal("expected active strict exclusivity rejection")
		}
	})

	t.Run("rejects envelope mismatch", func(t *testing.T) {
		tampered := entry
		tampered.Outputs = append([]orderOutput(nil), entry.Outputs...)
		tampered.Outputs[0].EndAmount = "199"
		if _, err := parseAndResolveOrder(tampered, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0)); err == nil {
			t.Fatal("expected envelope mismatch")
		}
	})

	t.Run("accepts exact-output input decay and cosigner override", func(t *testing.T) {
		inputDecay := order
		inputDecay.BaseInput.EndAmount = big.NewInt(140)
		inputDecay.BaseOutputs = []v2Output{{
			Token: tokenOut, StartAmount: big.NewInt(200), EndAmount: big.NewInt(200), Recipient: recipient,
		}}
		inputDecay.CosignerData.InputOverride = big.NewInt(80)
		inputHash, hashErr := v2OrderHash(inputDecay)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		encodedCosignerData, packErr := v2CosignerDataArguments.Pack(inputDecay.CosignerData)
		if packErr != nil {
			t.Fatal(packErr)
		}
		inputDecay.Cosignature, packErr = crypto.Sign(
			crypto.Keccak256(inputHash.Bytes(), encodedCosignerData),
			cosignerKey,
		)
		if packErr != nil {
			t.Fatal(packErr)
		}
		inputDecay.Cosignature[64] += 27
		body, packErr := v2OrderArguments.Pack(inputDecay)
		if packErr != nil {
			t.Fatal(packErr)
		}
		decayingEntry := entry
		decayingEntry.Outputs = append([]orderOutput(nil), entry.Outputs...)
		decayingEntry.EncodedOrder = hexutil.Encode(body)
		decayingEntry.OrderHash = inputHash.Hex()
		decayingEntry.Input.EndAmount = "140"
		decayingEntry.Outputs[0].StartAmount = "200"
		resolvedExactOutput, parseErr := parseAndResolveOrder(
			decayingEntry,
			orderSourceExclusiveV2,
			cfg,
			1,
			time.Unix(1_050, 0),
		)
		if parseErr != nil {
			t.Fatalf("exact-output order rejected: %v", parseErr)
		}
		if resolvedExactOutput.AmountIn.Cmp(big.NewInt(110)) != 0 ||
			resolvedExactOutput.AmountOut.Cmp(big.NewInt(200)) != 0 {
			t.Fatalf("resolved exact-output amounts = %s/%s, want 110/200",
				resolvedExactOutput.AmountIn, resolvedExactOutput.AmountOut)
		}
	})

	t.Run("rejects cosigner overrides outside signed base bounds", func(t *testing.T) {
		tests := []struct {
			name    string
			mutate  func(*v2Order)
			wantErr string
		}{
			{
				name: "input above base start",
				mutate: func(candidate *v2Order) {
					candidate.CosignerData.InputOverride = big.NewInt(101)
				},
				wantErr: "input override exceeds base start",
			},
			{
				name: "output below base start",
				mutate: func(candidate *v2Order) {
					candidate.CosignerData.OutputOverrides = []*big.Int{big.NewInt(219)}
				},
				wantErr: "output override 0 is below base start",
			},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				candidate := order
				test.mutate(&candidate)
				encodedCosignerData, packErr := v2CosignerDataArguments.Pack(candidate.CosignerData)
				if packErr != nil {
					t.Fatal(packErr)
				}
				candidate.Cosignature, packErr = crypto.Sign(
					crypto.Keccak256(hash.Bytes(), encodedCosignerData), cosignerKey,
				)
				if packErr != nil {
					t.Fatal(packErr)
				}
				candidate.Cosignature[64] += 27
				body, packErr := v2OrderArguments.Pack(candidate)
				if packErr != nil {
					t.Fatal(packErr)
				}
				candidateEntry := entry
				candidateEntry.EncodedOrder = hexutil.Encode(body)
				if _, parseErr := parseAndResolveOrder(
					candidateEntry, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0),
				); parseErr == nil || !strings.Contains(parseErr.Error(), test.wantErr) {
					t.Fatalf("err = %v, want %q", parseErr, test.wantErr)
				}
			})
		}
	})

	t.Run("accepts same-token fee outputs and sums their resolved amounts", func(t *testing.T) {
		multi := order
		multi.BaseOutputs = append(append([]v2Output(nil), order.BaseOutputs...), v2Output{
			Token: tokenOut, StartAmount: big.NewInt(20), EndAmount: big.NewInt(10), Recipient: recipient,
		})
		multi.CosignerData.OutputOverrides = append(
			append([]*big.Int(nil), order.CosignerData.OutputOverrides...), big.NewInt(0),
		)
		multiHash, hashErr := v2OrderHash(multi)
		if hashErr != nil {
			t.Fatal(hashErr)
		}
		encodedCosignerData, packErr := v2CosignerDataArguments.Pack(multi.CosignerData)
		if packErr != nil {
			t.Fatal(packErr)
		}
		multi.Cosignature, packErr = crypto.Sign(crypto.Keccak256(multiHash.Bytes(), encodedCosignerData), cosignerKey)
		if packErr != nil {
			t.Fatal(packErr)
		}
		multi.Cosignature[64] += 27
		body, packErr := v2OrderArguments.Pack(multi)
		if packErr != nil {
			t.Fatal(packErr)
		}
		multiEntry := entry
		multiEntry.EncodedOrder = hexutil.Encode(body)
		multiEntry.OrderHash = multiHash.Hex()
		multiEntry.Outputs = append(append([]orderOutput(nil), entry.Outputs...), orderOutput{
			Token: tokenOut.Hex(), StartAmount: "20", EndAmount: "10", Recipient: recipient.Hex(),
		})
		resolvedMulti, parseErr := parseAndResolveOrder(
			multiEntry, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0),
		)
		if parseErr != nil {
			t.Fatalf("multi-output order rejected: %v", parseErr)
		}
		if resolvedMulti.AmountOut.Cmp(big.NewInt(225)) != 0 {
			t.Fatalf("multi-output amount = %s, want 225", resolvedMulti.AmountOut)
		}
	})

	t.Run("rejects another filler", func(t *testing.T) {
		tamperedOrder := order
		tamperedOrder.CosignerData.ExclusiveFiller = recipient
		body, packErr := v2OrderArguments.Pack(tamperedOrder)
		if packErr != nil {
			t.Fatal(packErr)
		}
		tampered := entry
		tampered.EncodedOrder = hexutil.Encode(body)
		if _, err := parseAndResolveOrder(tampered, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0)); err == nil {
			t.Fatal("expected exclusive filler mismatch")
		}
	})

	t.Run("rejects zero cosigner", func(t *testing.T) {
		tamperedOrder := order
		tamperedOrder.Cosigner = common.Address{}
		body, packErr := v2OrderArguments.Pack(tamperedOrder)
		if packErr != nil {
			t.Fatal(packErr)
		}
		tampered := entry
		tampered.EncodedOrder = hexutil.Encode(body)
		if _, err := parseAndResolveOrder(
			tampered, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0),
		); err == nil || !strings.Contains(err.Error(), "cosigner must be non-zero") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("rejects backend hash mismatch", func(t *testing.T) {
		tampered := entry
		tampered.OrderHash = common.HexToHash("0x01").Hex()
		if _, err := parseAndResolveOrder(tampered, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0)); err == nil {
			t.Fatal("expected order hash mismatch")
		}
	})

	t.Run("rejects cosignature mismatch", func(t *testing.T) {
		tamperedOrder := order
		tamperedOrder.Cosignature = append([]byte(nil), order.Cosignature...)
		tamperedOrder.Cosignature[10] ^= 0xff
		body, packErr := v2OrderArguments.Pack(tamperedOrder)
		if packErr != nil {
			t.Fatal(packErr)
		}
		tampered := entry
		tampered.EncodedOrder = hexutil.Encode(body)
		if _, err := parseAndResolveOrder(tampered, orderSourceExclusiveV2, cfg, 1, time.Unix(1_050, 0)); err == nil {
			t.Fatal("expected cosignature mismatch")
		}
	})
}

func TestDecayMatchesDutchLinearRounding(t *testing.T) {
	if got := decay(big.NewInt(11), big.NewInt(0), 100, 103, 101); got.Cmp(big.NewInt(8)) != 0 {
		t.Fatalf("descending decay = %s, want 8", got)
	}
	if got := decay(big.NewInt(0), big.NewInt(11), 100, 103, 101); got.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("ascending decay = %s, want 3", got)
	}
}

func testExclusiveOrderEntry(t *testing.T, exclusiveFiller common.Address) (orderEntry, *Config) {
	t.Helper()
	executor := common.HexToAddress("0x2222222222222222222222222222222222222222")
	amount := big.NewInt(1)
	order := v2Order{
		Info: v2OrderInfo{
			Nonce: amount, Deadline: big.NewInt(1_200),
		},
		BaseInput: v2Input{StartAmount: amount, EndAmount: amount},
		BaseOutputs: []v2Output{{
			StartAmount: amount, EndAmount: amount,
		}},
		CosignerData: v2CosignerData{
			DecayStartTime: big.NewInt(1_000), DecayEndTime: big.NewInt(1_100),
			ExclusiveFiller: exclusiveFiller, ExclusivityOverrideBps: new(big.Int),
			InputOverride: new(big.Int), OutputOverrides: []*big.Int{new(big.Int)},
		},
	}
	hash, err := v2OrderHash(order)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := v2OrderArguments.Pack(order)
	if err != nil {
		t.Fatal(err)
	}
	return orderEntry{
		Type: orderTypeDutchV2, EncodedOrder: hexutil.Encode(encoded), Signature: "0x",
		OrderHash: hash.Hex(), OrderStatus: orderStatusOpen, ChainID: 1, QuoteID: "quote-1",
	}, &Config{Executor: executor}
}
