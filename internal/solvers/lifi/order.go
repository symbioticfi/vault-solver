package lifi

import (
	"encoding/json"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/bindings/lifi/inputsettler"
	"github.com/symbioticfi/vault-solver/api/lifiorder"
)

const (
	dutchAuctionContextType          byte = 0x01
	exclusiveDutchAuctionContextType byte = 0xe1
)

var errOrderForDifferentChain = errors.New("order is for a different chain")

type submittedOrderEvent struct {
	OrderType    string                        `json:"orderType"`
	Order        lifiorder.SubmitOrderDtoOrder `json:"order"`
	QuoteID      *string                       `json:"quoteId,omitempty"`
	InputSettler string                        `json:"inputSettler"`
	Meta         submittedOrderEventMeta       `json:"meta"`
}

type submittedOrderEventMeta struct {
	OrderStatus    string          `json:"orderStatus"`
	OrderID        string          `json:"orderIdentifier"`
	OnChainOrderID string          `json:"onChainOrderId"`
	QuoteID        json.RawMessage `json:"quoteId"`
}

type submittedOrder struct {
	QuoteID        string
	OrderStatus    string
	OrderID        string
	OnChainOrderID string
	dedupeKey      string
	processed      chan struct{}
	recoveryGen    uint64

	Order        inputsettler.StandardOrder
	InputSettler common.Address

	TokenIn      common.Address
	AmountIn     *big.Int
	TokenOut     common.Address
	OutputAmount *big.Int
	Output       inputsettler.MandateOutput
}

func isDutchAuctionContext(context []byte) bool {
	if len(context) == 0 {
		return false
	}
	return context[0] == dutchAuctionContextType || context[0] == exclusiveDutchAuctionContextType
}

type parsedStandardOrder struct {
	order        inputsettler.StandardOrder
	tokenIn      common.Address
	amountIn     *big.Int
	tokenOut     common.Address
	outputAmount *big.Int
	output       inputsettler.MandateOutput
}

type parsedOutput struct {
	output   inputsettler.MandateOutput
	tokenOut common.Address
	amount   *big.Int
}

func parseSubmittedOrder(data []byte, cfg *Config, chainID int64) (*submittedOrder, error) {
	var event submittedOrderEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, errors.Errorf("decode submit order dto: %w", err)
	}

	if !isFillableOrderStatus(event.Meta.OrderStatus) {
		return nil, errors.Errorf("unsupported order status %q", event.Meta.OrderStatus)
	}
	if !isOnChainOrderEvent(event) {
		if event.OrderType == "" {
			return nil, errors.New("missing orderType requires onChainOrderId and inputSettler")
		}
		return nil, errors.Errorf("unsupported non-onchain order type %q", event.OrderType)
	}

	inputSettler, err := parseAddress(event.InputSettler, "inputSettler")
	if err != nil {
		return nil, err
	}
	parsed, err := parseStandardOrder(event.Order)
	if err != nil {
		return nil, err
	}
	if err := validateOrderTarget(inputSettler, parsed.order, cfg, chainID); err != nil {
		return nil, err
	}
	dedupeKey, err := localOrderKey(parsed.order)
	if err != nil {
		return nil, err
	}
	return &submittedOrder{
		QuoteID:        eventQuoteID(event),
		OrderStatus:    event.Meta.OrderStatus,
		OrderID:        event.Meta.OrderID,
		OnChainOrderID: event.Meta.OnChainOrderID,
		dedupeKey:      dedupeKey,
		Order:          parsed.order,
		InputSettler:   inputSettler,
		TokenIn:        parsed.tokenIn,
		AmountIn:       parsed.amountIn,
		TokenOut:       parsed.tokenOut,
		OutputAmount:   new(big.Int).Set(parsed.outputAmount),
		Output:         parsed.output,
	}, nil
}

func localOrderKey(order inputsettler.StandardOrder) (string, error) {
	if order.Nonce == nil || order.OriginChainId == nil || len(order.Inputs) == 0 || len(order.Outputs) == 0 {
		return "", errors.New("incomplete order cannot be fingerprinted")
	}
	for _, input := range order.Inputs {
		if input[0] == nil || input[1] == nil {
			return "", errors.New("incomplete order input cannot be fingerprinted")
		}
	}
	for _, output := range order.Outputs {
		if output.ChainId == nil || output.Amount == nil {
			return "", errors.New("incomplete order output cannot be fingerprinted")
		}
	}
	data, err := lifiInputSettler.TryPackOrderIdentifier(order)
	if err != nil {
		return "", errors.Errorf("pack local order key: %w", err)
	}
	return crypto.Keccak256Hash(data).Hex(), nil
}

func isFillableOrderStatus(status string) bool {
	return status == "Signed" || status == "Delivered"
}

func isOnChainOrderType(orderType string) bool {
	normalized := strings.NewReplacer("-", "", "_", "", " ", "").Replace(strings.ToLower(orderType))
	switch normalized {
	case "onchainorder", "oifuseropenv0":
		return true
	default:
		return false
	}
}

func isOnChainOrderEvent(event submittedOrderEvent) bool {
	if event.OrderType == "" {
		return event.Meta.OnChainOrderID != "" && event.InputSettler != ""
	}
	return isOnChainOrderType(event.OrderType)
}

func parseStandardOrder(
	dto lifiorder.SubmitOrderDtoOrder,
) (*parsedStandardOrder, error) {
	user, err := parseAddress(dto.User, "order.user")
	if err != nil {
		return nil, err
	}
	inputOracle, err := parseAddress(dto.InputOracle, "order.inputOracle")
	if err != nil {
		return nil, err
	}
	if len(dto.Inputs) != 1 {
		return nil, errors.Errorf("order.inputs: expected 1 input, got %d", len(dto.Inputs))
	}
	if len(dto.Outputs) != 1 {
		return nil, errors.Errorf("order.outputs: expected 1 output, got %d", len(dto.Outputs))
	}

	nonce, err := parseUint(dto.Nonce, "order.nonce")
	if err != nil {
		return nil, err
	}
	originChainID, err := parseUint(dto.OriginChainId, "order.originChainId")
	if err != nil {
		return nil, err
	}
	expires, err := parseUint32(dto.Expires, "order.expires")
	if err != nil {
		return nil, err
	}
	fillDeadline, err := parseUint32(dto.FillDeadline, "order.fillDeadline")
	if err != nil {
		return nil, err
	}

	inputPair := dto.Inputs[0]
	if len(inputPair) != 2 {
		return nil, errors.Errorf("order.inputs[0]: expected [tokenId, amount], got %d values", len(inputPair))
	}
	tokenID, err := parseTupleUint(inputPair[0], "order.inputs[0][0]")
	if err != nil {
		return nil, err
	}
	tokenIn, err := tokenIDToAddress(tokenID, "order.inputs[0][0]")
	if err != nil {
		return nil, err
	}
	amountIn, err := parseTupleUint(inputPair[1], "order.inputs[0][1]")
	if err != nil {
		return nil, err
	}
	if amountIn.Sign() <= 0 {
		return nil, errors.New("order.inputs[0][1]: must be positive")
	}

	output, err := parseOutput(dto.Outputs[0])
	if err != nil {
		return nil, err
	}
	order := inputsettler.StandardOrder{
		User:          user,
		Nonce:         nonce,
		OriginChainId: originChainID,
		Expires:       expires,
		FillDeadline:  fillDeadline,
		InputOracle:   inputOracle,
		Inputs:        [][2]*big.Int{{new(big.Int).Set(tokenID), new(big.Int).Set(amountIn)}},
		Outputs:       []inputsettler.MandateOutput{output.output},
	}
	return &parsedStandardOrder{
		order:        order,
		tokenIn:      tokenIn,
		amountIn:     amountIn,
		tokenOut:     output.tokenOut,
		outputAmount: output.amount,
		output:       output.output,
	}, nil
}

func parseOutput(
	dto lifiorder.SubmitOrderDtoOrderOutputsInner,
) (*parsedOutput, error) {
	oracle, err := parseBytes32(dto.Oracle, "order.outputs[0].oracle")
	if err != nil {
		return nil, err
	}
	settler, err := parseBytes32(dto.Settler, "order.outputs[0].settler")
	if err != nil {
		return nil, err
	}
	tokenID, err := parseBytes32(dto.Token, "order.outputs[0].token")
	if err != nil {
		return nil, err
	}
	tokenOut, err := identifierAddress(tokenID, "order.outputs[0].token")
	if err != nil {
		return nil, err
	}
	recipientID, err := parseBytes32(dto.Recipient, "order.outputs[0].recipient")
	if err != nil {
		return nil, err
	}
	if _, err := identifierAddress(recipientID, "order.outputs[0].recipient"); err != nil {
		return nil, err
	}

	amountOut, err := parseUint(dto.Amount, "order.outputs[0].amount")
	if err != nil {
		return nil, err
	}
	if amountOut.Sign() <= 0 {
		return nil, errors.New("order.outputs[0].amount: must be positive")
	}
	outputChainID, err := parseUint(dto.ChainId, "order.outputs[0].chainId")
	if err != nil {
		return nil, err
	}

	callbackData, err := nullableHexBytes(dto.CallbackData, "order.outputs[0].callbackData")
	if err != nil {
		return nil, err
	}
	contextData, err := nullableHexBytes(dto.Context, "order.outputs[0].context")
	if err != nil {
		return nil, err
	}
	if len(callbackData) != 0 {
		return nil, errors.New("non-empty output callbackData is not supported")
	}

	output := inputsettler.MandateOutput{
		Oracle:       oracle,
		Settler:      settler,
		ChainId:      outputChainID,
		Token:        tokenID,
		Amount:       amountOut,
		Recipient:    recipientID,
		CallbackData: callbackData,
		Context:      contextData,
	}
	return &parsedOutput{output: output, tokenOut: tokenOut, amount: amountOut}, nil
}

func validateOrderTarget(
	inputSettler common.Address,
	order inputsettler.StandardOrder,
	cfg *Config,
	chainID int64,
) error {
	wantChainID := big.NewInt(chainID)
	outputChainID := order.Outputs[0].ChainId
	if order.OriginChainId.Cmp(wantChainID) != 0 || outputChainID.Cmp(wantChainID) != 0 {
		return errors.Errorf(
			"%w: originChainId %s, outputChainId %s, configuredChainId %d",
			errOrderForDifferentChain,
			order.OriginChainId,
			outputChainID,
			chainID,
		)
	}
	if inputSettler != cfg.InputSettler {
		return errors.Errorf("inputSettler %s does not match configured %s", inputSettler.Hex(), cfg.InputSettler.Hex())
	}
	if order.InputOracle != cfg.OutputSettler {
		return errors.Errorf(
			"order.inputOracle %s does not match outputSettler %s",
			order.InputOracle.Hex(),
			cfg.OutputSettler.Hex(),
		)
	}
	wantSettler := addressIdentifier(cfg.OutputSettler)
	if order.Outputs[0].Oracle != wantSettler {
		return errors.New("order.outputs[0].oracle does not match outputSettler")
	}
	if order.Outputs[0].Settler != wantSettler {
		return errors.New("order.outputs[0].settler does not match outputSettler")
	}
	return nil
}

func eventQuoteID(event submittedOrderEvent) string {
	if event.QuoteID != nil && *event.QuoteID != "" {
		return *event.QuoteID
	}
	if len(event.Meta.QuoteID) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(event.Meta.QuoteID, &s); err == nil {
		return s
	}
	return ""
}

func parseAddress(raw, field string) (common.Address, error) {
	if !common.IsHexAddress(raw) {
		return common.Address{}, errors.Errorf("%s: invalid address %q", field, raw)
	}
	addr := common.HexToAddress(raw)
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address", field)
	}
	return addr, nil
}

func parseUint32(raw, field string) (uint32, error) {
	n, err := parseUint(raw, field)
	if err != nil {
		return 0, err
	}
	if !n.IsUint64() || n.Uint64() > math.MaxUint32 {
		return 0, errors.Errorf("%s: overflows uint32", field)
	}
	return uint32(n.Uint64()), nil
}

func parseTupleUint(raw any, field string) (*big.Int, error) {
	value, ok := raw.(string)
	if !ok {
		return nil, errors.Errorf("%s: expected decimal string, got %T", field, raw)
	}
	return parseUint(value, field)
}

func parseUint(raw, field string) (*big.Int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.Errorf("%s: empty integer", field)
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid uint %q", field, raw)
	}
	return n, nil
}

func parseBytes32(raw, field string) ([32]byte, error) {
	b, err := decodeHexBytes(raw, field)
	if err != nil {
		return [32]byte{}, err
	}
	if len(b) != 32 {
		return [32]byte{}, errors.Errorf("%s: expected 32 bytes, got %d", field, len(b))
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}

func decodeHexBytes(raw, field string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.Errorf("%s: empty hex", field)
	}
	if !strings.HasPrefix(raw, "0x") && !strings.HasPrefix(raw, "0X") {
		raw = "0x" + raw
	}
	out, err := hexutil.Decode(raw)
	if err != nil {
		return nil, errors.Errorf("%s: invalid hex: %w", field, err)
	}
	return out, nil
}

func nullableHexBytes(value lifiorder.NullableString, field string) ([]byte, error) {
	if !value.IsSet() || value.Get() == nil || *value.Get() == "" {
		return nil, nil
	}
	return decodeHexBytes(*value.Get(), field)
}

func tokenIDToAddress(n *big.Int, field string) (common.Address, error) {
	addr := common.BytesToAddress(n.Bytes())
	roundTrip := new(big.Int).SetBytes(addr.Bytes())
	if addr == (common.Address{}) || roundTrip.Cmp(n) != 0 {
		return common.Address{}, errors.Errorf("%s: not a clean address identifier", field)
	}
	return addr, nil
}

func addressIdentifier(addr common.Address) [32]byte {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return out
}

func identifierAddress(id [32]byte, field string) (common.Address, error) {
	addr := common.BytesToAddress(id[12:])
	if addr == (common.Address{}) {
		return common.Address{}, errors.Errorf("%s: zero address identifier", field)
	}
	if addressIdentifier(addr) != id {
		return common.Address{}, errors.Errorf("%s: not a clean address identifier", field)
	}
	return addr, nil
}
