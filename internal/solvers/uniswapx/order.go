package uniswapx

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/trace"
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
	// span is the uniswapx.order.track span the poll accepted this order under. It rides through the
	// orders channel so the fill this order produces continues that trace (spec §9.4).
	span trace.SpanContext
	// quoteTraceID is the trace of the quote this order was awarded from, when the poll linked one.
	// It rides along because it cannot be derived from span: the quote lives in its own trace.
	quoteTraceID string
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

func parseAndResolveOrder(
	entry orderEntry,
	source orderSource,
	cfg *Config,
	chainID int64,
	now time.Time,
) (*resolvedOrder, error) {
	return parseAndResolveV2Order(entry, source, cfg, chainID, now)
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
	order, err := decodeV2Order(encoded)
	if err != nil {
		return exclusiveObligation{}, err
	}
	if order.CosignerData.ExclusiveFiller != cfg.Executor {
		return exclusiveObligation{}, errors.Errorf(
			"%w: got %s",
			errDifferentExclusiveFiller,
			order.CosignerData.ExclusiveFiller.Hex(),
		)
	}
	deadline, ok := uint64Value(order.CosignerData.DecayStartTime)
	if !ok || deadline == 0 {
		return exclusiveObligation{}, errors.Errorf("exclusive deadline (decay start) %v is zero or exceeds uint64",
			order.CosignerData.DecayStartTime)
	}
	hash, err := matchingOrderHash(entry, order)
	if err != nil {
		return exclusiveObligation{}, err
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
	if entry.Type != orderTypeDutchV2 {
		return nil, errors.Errorf("unsupported order type %q", entry.Type)
	}
	if entry.OrderStatus != orderStatusOpen {
		return nil, errors.Errorf("order status %q is not %q", entry.OrderStatus, orderStatusOpen)
	}
	if entry.ChainID != chainID {
		return nil, errors.Errorf("order chain id %d does not match %d", entry.ChainID, chainID)
	}
	encoded, err := hexutil.Decode(entry.EncodedOrder)
	if err != nil {
		return nil, errors.Errorf("encodedOrder: %w", err)
	}
	signature, err := hexutil.Decode(entry.Signature)
	if err != nil {
		return nil, errors.Errorf("signature is malformed: %w", err)
	}
	if len(signature) == 0 {
		return nil, errors.New("signature is empty")
	}
	order, err := decodeV2Order(encoded)
	if err != nil {
		return nil, err
	}
	if order.Info.Reactor != cfg.Reactor {
		return nil, errors.Errorf("order reactor %s is not configured reactor %s", order.Info.Reactor.Hex(), cfg.Reactor.Hex())
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
	if len(order.BaseOutputs) == 0 {
		return nil, errors.New("order has no outputs")
	}
	if len(order.CosignerData.OutputOverrides) != len(order.BaseOutputs) {
		return nil, errors.Errorf("order has %d output overrides for %d outputs",
			len(order.CosignerData.OutputOverrides), len(order.BaseOutputs))
	}
	if len(order.Cosignature) != crypto.SignatureLength {
		return nil, errors.Errorf("cosignature is %d bytes, want %d", len(order.Cosignature), crypto.SignatureLength)
	}
	if order.Info.Swapper == (common.Address{}) {
		return nil, errors.New("swapper must be non-zero")
	}
	if !cfg.TokenPolicy.Allows(order.BaseInput.Token) {
		return nil, errors.Errorf("input token %s is not allowed by token policy", order.BaseInput.Token.Hex())
	}
	deadline, ok := uint32Value(order.Info.Deadline)
	if !ok {
		return nil, errors.Errorf("order deadline %v exceeds uint32", order.Info.Deadline)
	}
	if int64(deadline) <= now.Unix() {
		return nil, errors.Errorf("order deadline %d is not after now %d", deadline, now.Unix())
	}
	start, startOK := uint64Value(order.CosignerData.DecayStartTime)
	end, endOK := uint64Value(order.CosignerData.DecayEndTime)
	switch {
	case !startOK || !endOK:
		return nil, errors.Errorf("decay window %v-%v exceeds uint64",
			order.CosignerData.DecayStartTime, order.CosignerData.DecayEndTime)
	case end <= start:
		return nil, errors.Errorf("decay end %d is not after decay start %d", end, start)
	case uint64(deadline) < end:
		return nil, errors.Errorf("order deadline %d is before decay end %d", deadline, end)
	}
	if order.CosignerData.InputOverride != nil && order.CosignerData.InputOverride.Sign() > 0 &&
		order.CosignerData.InputOverride.Cmp(order.BaseInput.StartAmount) > 0 {
		return nil, errors.Errorf("cosigner input override %s exceeds base start amount %s",
			order.CosignerData.InputOverride, order.BaseInput.StartAmount)
	}
	inputStart := originalIfZero(order.CosignerData.InputOverride, order.BaseInput.StartAmount)
	switch {
	case inputStart.Sign() <= 0:
		return nil, errors.Errorf("order input start amount %s is not positive", inputStart)
	case order.BaseInput.EndAmount.Sign() <= 0:
		return nil, errors.Errorf("order input end amount %s is not positive", order.BaseInput.EndAmount)
	case inputStart.Cmp(order.BaseInput.EndAmount) > 0:
		return nil, errors.Errorf("order input start amount %s exceeds end amount %s", inputStart, order.BaseInput.EndAmount)
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
			return nil, errors.Errorf("order has strict exclusivity for %s until %d (now %d) with no override bps",
				order.CosignerData.ExclusiveFiller.Hex(), start, now.Unix())
		}
	}
	tokenOut := order.BaseOutputs[0].Token
	if tokenOut == (common.Address{}) {
		return nil, errors.New("native output token is unsupported")
	}
	if tokenOut == order.BaseInput.Token {
		return nil, errors.Errorf("output token %s equals the input token", tokenOut.Hex())
	}
	amountOut := new(big.Int)
	for i, output := range order.BaseOutputs {
		if override := order.CosignerData.OutputOverrides[i]; override != nil && override.Sign() > 0 &&
			override.Cmp(output.StartAmount) < 0 {
			return nil, errors.Errorf("cosigner output override %d (%s) is below base start amount %s",
				i, override, output.StartAmount)
		}
		outputStart := originalIfZero(order.CosignerData.OutputOverrides[i], output.StartAmount)
		switch {
		case output.Token != tokenOut:
			return nil, errors.Errorf("output %d token %s differs from first output token %s", i, output.Token.Hex(), tokenOut.Hex())
		case output.Recipient == (common.Address{}):
			return nil, errors.Errorf("output %d recipient is the zero address", i)
		case outputStart.Sign() <= 0:
			return nil, errors.Errorf("output %d start amount %s is not positive", i, outputStart)
		case outputStart.Cmp(output.EndAmount) < 0:
			return nil, errors.Errorf("output %d start amount %s is below end amount %s", i, outputStart, output.EndAmount)
		}
		resolved := decay(outputStart, output.EndAmount, start, end, uint64(now.Unix()))
		if applyOverride {
			resolved = applyExclusiveOverride(resolved, order.CosignerData.ExclusivityOverrideBps)
		}
		amountOut.Add(amountOut, resolved)
	}
	if mismatch := orderEnvelopeMismatch(entry, order); mismatch != "" {
		return nil, errors.Errorf("order envelope does not match encoded order: %s", mismatch)
	}
	hash, err := matchingOrderHash(entry, order)
	if err != nil {
		return nil, err
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

func decodeV2Order(encoded []byte) (v2Order, error) {
	decoded, err := v2OrderArguments.Unpack(encoded)
	if err != nil {
		return v2Order{}, errors.Errorf("decode V2 order: %w", err)
	}
	if len(decoded) != 1 {
		return v2Order{}, errors.Errorf("decode V2 order: got %d values, want 1", len(decoded))
	}
	return *abi.ConvertType(decoded[0], new(v2Order)).(*v2Order), nil
}

// matchingOrderHash hashes the encoded order and checks it against the backend's orderHash.
func matchingOrderHash(entry orderEntry, order v2Order) (common.Hash, error) {
	hash, err := v2OrderHash(order)
	if err != nil {
		return common.Hash{}, err
	}
	backendHash, err := parseHash(entry.OrderHash)
	if err != nil {
		return common.Hash{}, err
	}
	if backendHash != hash {
		return common.Hash{}, errors.Errorf("orderHash %s does not match encoded order hash %s", backendHash.Hex(), hash.Hex())
	}
	return hash, nil
}

// orderEnvelopeMismatch names the first envelope field that differs from the encoded order, or "".
func orderEnvelopeMismatch(entry orderEntry, order v2Order) string {
	if !sameAddress(entry.Input.Token, order.BaseInput.Token) {
		return fmt.Sprintf("input token %q, encoded %s", entry.Input.Token, order.BaseInput.Token.Hex())
	}
	if len(entry.Outputs) != len(order.BaseOutputs) {
		return fmt.Sprintf("%d outputs, encoded %d", len(entry.Outputs), len(order.BaseOutputs))
	}
	if entry.Input.StartAmount != order.BaseInput.StartAmount.String() {
		return fmt.Sprintf("input start amount %q, encoded %s", entry.Input.StartAmount, order.BaseInput.StartAmount)
	}
	if entry.Input.EndAmount != order.BaseInput.EndAmount.String() {
		return fmt.Sprintf("input end amount %q, encoded %s", entry.Input.EndAmount, order.BaseInput.EndAmount)
	}
	for i, output := range order.BaseOutputs {
		got := entry.Outputs[i]
		switch {
		case !sameAddress(got.Token, output.Token):
			return fmt.Sprintf("output %d token %q, encoded %s", i, got.Token, output.Token.Hex())
		case !sameAddress(got.Recipient, output.Recipient):
			return fmt.Sprintf("output %d recipient %q, encoded %s", i, got.Recipient, output.Recipient.Hex())
		case got.StartAmount != output.StartAmount.String():
			return fmt.Sprintf("output %d start amount %q, encoded %s", i, got.StartAmount, output.StartAmount)
		case got.EndAmount != output.EndAmount.String():
			return fmt.Sprintf("output %d end amount %q, encoded %s", i, got.EndAmount, output.EndAmount)
		}
	}
	return ""
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
	if len(signature) != crypto.SignatureLength {
		return errors.Errorf("cosignature is %d bytes, want %d", len(signature), crypto.SignatureLength)
	}
	if v := signature[64]; v < 27 || v > 28 {
		return errors.Errorf("cosignature recovery id %d is not 27 or 28", v)
	}
	signature[64] -= 27
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:64])
	if !crypto.ValidateSignatureValues(signature[64], r, s, true) {
		return errors.New("cosignature is not canonical")
	}
	publicKey, err := crypto.SigToPub(digest.Bytes(), signature)
	if err != nil {
		return errors.Errorf("recover cosignature signer: %w", err)
	}
	if signer := crypto.PubkeyToAddress(*publicKey); signer != order.Cosigner {
		return errors.Errorf("cosignature signer %s is not order cosigner %s", signer.Hex(), order.Cosigner.Hex())
	}
	return nil
}

func sameAddress(value string, expected common.Address) bool {
	return common.IsHexAddress(value) && common.HexToAddress(value) == expected
}

func parseHash(value string) (common.Hash, error) {
	b, err := hexutil.Decode(value)
	if err != nil {
		return common.Hash{}, errors.Errorf("orderHash %q is malformed: %w", value, err)
	}
	if len(b) != common.HashLength {
		return common.Hash{}, errors.Errorf("orderHash %q is %d bytes, want %d", value, len(b), common.HashLength)
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
