package uniswapx

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
)

const (
	v2OrderInfoType = "OrderInfo(address reactor,address swapper,uint256 nonce,uint256 deadline,address additionalValidationContract,bytes additionalValidationData)"
	v2OutputType    = "DutchOutput(address token,uint256 startAmount,uint256 endAmount,address recipient)"
	v2WitnessType   = "V2DutchOrder(OrderInfo info,address cosigner,address baseInputToken,uint256 baseInputStartAmount,uint256 baseInputEndAmount,DutchOutput[] baseOutputs)" +
		v2OutputType + v2OrderInfoType
)

type orderSource string

const (
	orderSourceExclusiveV2 orderSource = "exclusive-v2"
	orderSourcePublicV2    orderSource = "public-v2"
	orderStatusOpen                    = "open"
	orderTypeDutchV2                   = "Dutch_V2"
)

type v2OrderInfo struct {
	Reactor                      common.Address
	Swapper                      common.Address
	Nonce                        *big.Int
	Deadline                     *big.Int
	AdditionalValidationContract common.Address
	AdditionalValidationData     []byte
}

type v2Input struct {
	Token       common.Address
	StartAmount *big.Int
	EndAmount   *big.Int
}

type v2Output struct {
	Token       common.Address
	StartAmount *big.Int
	EndAmount   *big.Int
	Recipient   common.Address
}

type v2CosignerData struct {
	DecayStartTime         *big.Int
	DecayEndTime           *big.Int
	ExclusiveFiller        common.Address
	ExclusivityOverrideBps *big.Int
	InputOverride          *big.Int
	OutputOverrides        []*big.Int
}

type v2Order struct {
	Info         v2OrderInfo
	Cosigner     common.Address
	BaseInput    v2Input
	BaseOutputs  []v2Output
	CosignerData v2CosignerData
	Cosignature  []byte
}

type resolvedOrder struct {
	Encoded        []byte
	Signature      []byte
	Hash           common.Hash
	QuoteID        string
	Source         orderSource
	Executor       common.Address
	TokenIn        common.Address
	TokenOut       common.Address
	AmountIn       *big.Int
	AmountOut      *big.Int
	Deadline       uint32
	ExclusiveUntil uint64
}

var (
	v2OrderArguments            = mustV2OrderArguments()
	v2CosignerDataArguments     = mustV2CosignerDataArguments()
	v2HashArguments             = mustV2HashArguments()
	errDifferentExclusiveFiller = errors.New("order is exclusive to another filler")
)

type v2HashABIs struct {
	info    abi.Arguments
	output  abi.Arguments
	witness abi.Arguments
}

func mustV2OrderArguments() abi.Arguments {
	components := []abi.ArgumentMarshaling{
		{Name: "info", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "reactor", Type: "address"}, {Name: "swapper", Type: "address"},
			{Name: "nonce", Type: "uint256"}, {Name: "deadline", Type: "uint256"},
			{Name: "additionalValidationContract", Type: "address"},
			{Name: "additionalValidationData", Type: "bytes"},
		}},
		{Name: "cosigner", Type: "address"},
		{Name: "baseInput", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "token", Type: "address"}, {Name: "startAmount", Type: "uint256"}, {Name: "endAmount", Type: "uint256"},
		}},
		{Name: "baseOutputs", Type: "tuple[]", Components: []abi.ArgumentMarshaling{
			{Name: "token", Type: "address"}, {Name: "startAmount", Type: "uint256"},
			{Name: "endAmount", Type: "uint256"}, {Name: "recipient", Type: "address"},
		}},
		{Name: "cosignerData", Type: "tuple", Components: []abi.ArgumentMarshaling{
			{Name: "decayStartTime", Type: "uint256"}, {Name: "decayEndTime", Type: "uint256"},
			{Name: "exclusiveFiller", Type: "address"}, {Name: "exclusivityOverrideBps", Type: "uint256"},
			{Name: "inputOverride", Type: "uint256"}, {Name: "outputOverrides", Type: "uint256[]"},
		}},
		{Name: "cosignature", Type: "bytes"},
	}
	t, err := abi.NewType("tuple", "V2DutchOrder", components)
	if err != nil {
		panic(err)
	}
	return abi.Arguments{{Type: t}}
}

func mustV2CosignerDataArguments() abi.Arguments {
	t, err := abi.NewType("tuple", "CosignerData", []abi.ArgumentMarshaling{
		{Name: "decayStartTime", Type: "uint256"}, {Name: "decayEndTime", Type: "uint256"},
		{Name: "exclusiveFiller", Type: "address"}, {Name: "exclusivityOverrideBps", Type: "uint256"},
		{Name: "inputOverride", Type: "uint256"}, {Name: "outputOverrides", Type: "uint256[]"},
	})
	if err != nil {
		panic(err)
	}
	return abi.Arguments{{Type: t}}
}

func mustV2HashArguments() v2HashABIs {
	bytes32Type, err := abi.NewType("bytes32", "", nil)
	if err != nil {
		panic(err)
	}
	addressType, err := abi.NewType("address", "", nil)
	if err != nil {
		panic(err)
	}
	uintType, err := abi.NewType("uint256", "", nil)
	if err != nil {
		panic(err)
	}
	return v2HashABIs{
		info: abi.Arguments{
			{Type: bytes32Type}, {Type: addressType}, {Type: addressType}, {Type: uintType},
			{Type: uintType}, {Type: addressType}, {Type: bytes32Type},
		},
		output: abi.Arguments{
			{Type: bytes32Type}, {Type: addressType}, {Type: uintType}, {Type: uintType}, {Type: addressType},
		},
		witness: abi.Arguments{
			{Type: bytes32Type}, {Type: bytes32Type}, {Type: addressType}, {Type: addressType},
			{Type: uintType}, {Type: uintType}, {Type: bytes32Type},
		},
	}
}

func exclusiveObligationFromEntry(
	entry orderEntry,
	cfg *Config,
	chainID int64,
) (exclusiveObligation, error) {
	if entry.Type != orderTypeDutchV2 {
		return exclusiveObligation{}, errors.Errorf("unsupported order type %q", entry.Type)
	}
	if entry.ChainID != chainID {
		return exclusiveObligation{}, errors.Errorf("order chain id %d does not match %d", entry.ChainID, chainID)
	}
	encoded, err := hexutil.Decode(entry.EncodedOrder)
	if err != nil {
		return exclusiveObligation{}, errors.Errorf("encodedOrder: %w", err)
	}
	decoded, err := v2OrderArguments.Unpack(encoded)
	if err != nil || len(decoded) != 1 {
		return exclusiveObligation{}, errors.Errorf("decode V2 order: %w", err)
	}
	order := *abi.ConvertType(decoded[0], new(v2Order)).(*v2Order)
	if order.CosignerData.ExclusiveFiller != cfg.Executor {
		return exclusiveObligation{}, errors.Errorf(
			"%w: got %s",
			errDifferentExclusiveFiller,
			order.CosignerData.ExclusiveFiller.Hex(),
		)
	}
	deadline, ok := uint64Value(order.CosignerData.DecayStartTime)
	if !ok || deadline == 0 {
		return exclusiveObligation{}, errors.New("invalid exclusive deadline")
	}
	hash, err := v2OrderHash(order)
	if err != nil {
		return exclusiveObligation{}, err
	}
	backendHash, err := parseHash(entry.OrderHash)
	if err != nil || backendHash != hash {
		return exclusiveObligation{}, errors.New("orderHash does not match encoded order")
	}
	return exclusiveObligation{hash: hash, deadline: time.Unix(int64(deadline), 0)}, nil
}

func parseAndResolveV2Order(
	entry orderEntry,
	source orderSource,
	cfg *Config,
	chainID int64,
	now time.Time,
) (*resolvedOrder, error) {
	if entry.Type != orderTypeDutchV2 || entry.OrderStatus != orderStatusOpen {
		return nil, errors.Errorf("unsupported order type/status %q/%q", entry.Type, entry.OrderStatus)
	}
	if entry.ChainID != chainID {
		return nil, errors.Errorf("order chain id %d does not match %d", entry.ChainID, chainID)
	}
	encoded, err := hexutil.Decode(entry.EncodedOrder)
	if err != nil {
		return nil, errors.Errorf("encodedOrder: %w", err)
	}
	signature, err := hexutil.Decode(entry.Signature)
	if err != nil || len(signature) == 0 {
		return nil, errors.New("signature is missing or malformed")
	}
	decoded, err := v2OrderArguments.Unpack(encoded)
	if err != nil || len(decoded) != 1 {
		return nil, errors.Errorf("decode V2 order: %w", err)
	}
	order := *abi.ConvertType(decoded[0], new(v2Order)).(*v2Order)
	if order.Info.Reactor != cfg.Reactor {
		return nil, errors.New("reactor mismatch")
	}
	if order.Cosigner == (common.Address{}) {
		return nil, errors.New("cosigner must be non-zero")
	}
	if source == orderSourceExclusiveV2 && order.CosignerData.ExclusiveFiller != cfg.Executor {
		return nil, errors.Errorf("exclusive filler mismatch: got %s", order.CosignerData.ExclusiveFiller.Hex())
	}
	if source == orderSourcePublicV2 && order.CosignerData.ExclusiveFiller == cfg.Executor {
		return nil, errors.New("order belongs to the exclusive V2 source")
	}
	if len(order.Cosignature) != 65 || len(order.BaseOutputs) == 0 ||
		len(order.CosignerData.OutputOverrides) != len(order.BaseOutputs) {
		return nil, errors.New("order must have outputs, one override per output, and a 65-byte cosignature")
	}
	if order.Info.Swapper == (common.Address{}) {
		return nil, errors.New("swapper must be non-zero")
	}
	if !cfg.TokenPolicy.Allows(order.BaseInput.Token) {
		return nil, errors.New("input token rejected by token policy")
	}
	deadline, ok := uint32Value(order.Info.Deadline)
	if !ok || int64(deadline) <= now.Unix() {
		return nil, errors.New("order deadline is expired or exceeds uint32")
	}
	start, startOK := uint64Value(order.CosignerData.DecayStartTime)
	end, endOK := uint64Value(order.CosignerData.DecayEndTime)
	if !startOK || !endOK || end <= start || order.Info.Deadline.Cmp(order.CosignerData.DecayEndTime) < 0 {
		return nil, errors.New("invalid decay window")
	}
	if order.CosignerData.InputOverride != nil && order.CosignerData.InputOverride.Sign() > 0 &&
		order.CosignerData.InputOverride.Cmp(order.BaseInput.StartAmount) > 0 {
		return nil, errors.New("cosigner input override exceeds base start amount")
	}
	inputStart := originalIfZero(order.CosignerData.InputOverride, order.BaseInput.StartAmount)
	if inputStart.Sign() <= 0 || order.BaseInput.EndAmount.Sign() <= 0 || inputStart.Cmp(order.BaseInput.EndAmount) > 0 {
		return nil, errors.New("order input must be positive and ascend or remain fixed")
	}
	amountIn := decay(inputStart, order.BaseInput.EndAmount, start, end, uint64(now.Unix()))
	applyOverride := source == orderSourcePublicV2 && requiresExclusiveOverride(
		order.CosignerData.ExclusiveFiller,
		cfg.Executor,
		start,
		uint64(now.Unix()),
	)
	if applyOverride {
		if order.CosignerData.ExclusivityOverrideBps == nil || order.CosignerData.ExclusivityOverrideBps.Sign() == 0 {
			return nil, errors.New("order has active strict exclusivity")
		}
	}
	tokenOut := order.BaseOutputs[0].Token
	if tokenOut == (common.Address{}) || tokenOut == order.BaseInput.Token {
		return nil, errors.New("native and identical output tokens are unsupported")
	}
	amountOut := new(big.Int)
	for i, output := range order.BaseOutputs {
		if override := order.CosignerData.OutputOverrides[i]; override != nil && override.Sign() > 0 &&
			override.Cmp(output.StartAmount) < 0 {
			return nil, errors.Errorf("cosigner output override %d is below base start amount", i)
		}
		outputStart := originalIfZero(order.CosignerData.OutputOverrides[i], output.StartAmount)
		if output.Token != tokenOut || output.Recipient == (common.Address{}) ||
			outputStart.Sign() <= 0 || outputStart.Cmp(output.EndAmount) < 0 {
			return nil, errors.New("outputs must use one token, non-zero recipients, and descend or remain fixed")
		}
		resolved := decay(outputStart, output.EndAmount, start, end, uint64(now.Unix()))
		if applyOverride {
			resolved = applyExclusiveOverride(resolved, order.CosignerData.ExclusivityOverrideBps)
		}
		amountOut.Add(amountOut, resolved)
	}
	if !sameOrderEnvelope(entry, order) {
		return nil, errors.New("order envelope does not match encoded order")
	}
	hash, err := v2OrderHash(order)
	if err != nil {
		return nil, err
	}
	backendHash, err := parseHash(entry.OrderHash)
	if err != nil || backendHash != hash {
		return nil, errors.New("orderHash does not match encoded order")
	}
	if err := validateCosignature(order, hash); err != nil {
		return nil, err
	}
	return &resolvedOrder{
		Encoded: encoded, Signature: signature, Hash: hash, QuoteID: entry.QuoteID,
		Source: source, Executor: cfg.Executor,
		TokenIn: order.BaseInput.Token, TokenOut: tokenOut,
		AmountIn: amountIn, AmountOut: amountOut, Deadline: deadline, ExclusiveUntil: start,
	}, nil
}

func originalIfZero(override, original *big.Int) *big.Int {
	if override == nil || override.Sign() == 0 {
		return new(big.Int).Set(original)
	}
	return new(big.Int).Set(override)
}

func decay(startAmount, endAmount *big.Int, start, end, now uint64) *big.Int {
	if now <= start {
		return new(big.Int).Set(startAmount)
	}
	if now >= end {
		return new(big.Int).Set(endAmount)
	}
	delta := new(big.Int).Sub(endAmount, startAmount)
	elapsed := new(big.Int).SetUint64(now - start)
	duration := new(big.Int).SetUint64(end - start)
	delta.Mul(delta, elapsed).Quo(delta, duration)
	return new(big.Int).Add(startAmount, delta)
}

func sameOrderEnvelope(entry orderEntry, order v2Order) bool {
	if !sameAddress(entry.Input.Token, order.BaseInput.Token) || len(entry.Outputs) != len(order.BaseOutputs) {
		return false
	}
	if entry.Input.StartAmount != order.BaseInput.StartAmount.String() ||
		entry.Input.EndAmount != order.BaseInput.EndAmount.String() {
		return false
	}
	for i, output := range order.BaseOutputs {
		if !sameAddress(entry.Outputs[i].Token, output.Token) ||
			!sameAddress(entry.Outputs[i].Recipient, output.Recipient) ||
			entry.Outputs[i].StartAmount != output.StartAmount.String() ||
			entry.Outputs[i].EndAmount != output.EndAmount.String() {
			return false
		}
	}
	return true
}

func requiresExclusiveOverride(exclusive, executor common.Address, exclusivityEnd, now uint64) bool {
	return exclusive != (common.Address{}) && exclusive != executor && now <= exclusivityEnd
}

func applyExclusiveOverride(amount, bps *big.Int) *big.Int {
	numerator := new(big.Int).Mul(amount, new(big.Int).Add(big.NewInt(10_000), bps))
	numerator.Add(numerator, big.NewInt(9_999))
	return numerator.Quo(numerator, big.NewInt(10_000))
}

func v2OrderHash(order v2Order) (common.Hash, error) {
	info, err := v2HashArguments.info.Pack(
		crypto.Keccak256Hash([]byte(v2OrderInfoType)), order.Info.Reactor, order.Info.Swapper,
		order.Info.Nonce, order.Info.Deadline, order.Info.AdditionalValidationContract,
		crypto.Keccak256Hash(order.Info.AdditionalValidationData),
	)
	if err != nil {
		return common.Hash{}, errors.Errorf("hash order info: %w", err)
	}
	outputHashes := make([]byte, 0, common.HashLength*len(order.BaseOutputs))
	for _, output := range order.BaseOutputs {
		encodedOutput, packErr := v2HashArguments.output.Pack(
			crypto.Keccak256Hash([]byte(v2OutputType)), output.Token,
			output.StartAmount, output.EndAmount, output.Recipient,
		)
		if packErr != nil {
			return common.Hash{}, errors.Errorf("hash order output: %w", packErr)
		}
		outputHash := crypto.Keccak256Hash(encodedOutput)
		outputHashes = append(outputHashes, outputHash.Bytes()...)
	}
	witness, err := v2HashArguments.witness.Pack(
		crypto.Keccak256Hash([]byte(v2WitnessType)), crypto.Keccak256Hash(info), order.Cosigner,
		order.BaseInput.Token, order.BaseInput.StartAmount, order.BaseInput.EndAmount,
		crypto.Keccak256Hash(outputHashes),
	)
	if err != nil {
		return common.Hash{}, errors.Errorf("hash V2 order: %w", err)
	}
	return crypto.Keccak256Hash(witness), nil
}

func validateCosignature(order v2Order, orderHash common.Hash) error {
	encodedData, err := v2CosignerDataArguments.Pack(order.CosignerData)
	if err != nil {
		return errors.Errorf("encode cosigner data: %w", err)
	}
	digest := crypto.Keccak256Hash(orderHash.Bytes(), encodedData)
	signature := append([]byte(nil), order.Cosignature...)
	if len(signature) != crypto.SignatureLength || signature[64] < 27 || signature[64] > 28 {
		return errors.New("cosignature has invalid recovery id")
	}
	signature[64] -= 27
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	if !crypto.ValidateSignatureValues(signature[64], r, s, true) {
		return errors.New("cosignature is not canonical")
	}
	publicKey, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil || crypto.PubkeyToAddress(*publicKey) != order.Cosigner {
		return errors.New("cosignature signer mismatch")
	}
	return nil
}

func sameAddress(value string, expected common.Address) bool {
	return common.IsHexAddress(value) && common.HexToAddress(value) == expected
}

func parseHash(value string) (common.Hash, error) {
	b, err := hexutil.Decode(value)
	if err != nil || len(b) != common.HashLength {
		return common.Hash{}, errors.New("orderHash is malformed")
	}
	return common.BytesToHash(b), nil
}

func uint32Value(value *big.Int) (uint32, bool) {
	if value == nil || !value.IsUint64() || value.Uint64() > uint64(^uint32(0)) {
		return 0, false
	}
	return uint32(value.Uint64()), true
}

func uint64Value(value *big.Int) (uint64, bool) {
	if value == nil || !value.IsUint64() {
		return 0, false
	}
	return value.Uint64(), true
}
