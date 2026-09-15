package lifi

import (
	"encoding/json"
	"fmt"
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

var (
	errOrderForDifferentChain = errors.New("order is for a different chain")
	// errOrderUnsupported marks order kinds the feed carries but this solver never fills (non-EVM
	// submissions, non-fillable statuses). Expected traffic, not a malformed message.
	errOrderUnsupported       = errors.New("unsupported order")
	errNativeInputUnsupported = errors.Errorf("native input is not supported: %w", errOrderUnsupported)
)

type submittedOrderEvent struct {
	OrderType    string                  `json:"orderType"`
	Order        json.RawMessage         `json:"order"`
	QuoteID      *string                 `json:"quoteId,omitempty"`
	InputSettler string                  `json:"inputSettler"`
	Meta         submittedOrderEventMeta `json:"meta"`
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
		return nil, decodeOrderError(orderRawObject(event.Order), err)
	}

	if !isFillableOrderStatus(event.Meta.OrderStatus) {
		return nil, orderFieldError("lifi_unsupported_status", "meta.orderStatus", "unsupported order status", errOrderUnsupported)
	}
	if !isOnChainOrderEvent(event) {
		if event.OrderType == "" {
			return nil, orderFieldError("lifi_missing_order_type", "orderType", "missing orderType requires onChainOrderId and inputSettler", nil)
		}
		return nil, orderFieldError("lifi_unsupported_order_type", "orderType", "unsupported non-onchain order type", errOrderUnsupported)
	}

	// Classify by chain before reading any address: the feed carries every network LI.FI serves
	// (a Solana settler is a base58 program id) and cross-chain orders, and this solver only fills
	// same-chain orders on its configured chain.
	rawOrder := orderRawObject(event.Order)
	if err := validateOrderChain(rawOrder["originChainId"], "order.originChainId", chainID); err != nil {
		return nil, err
	}
	// A foreign output chain is also outside our scope, even if its settler is non-EVM.
	outputChainErr := validateOrderOutputChains(rawOrder["outputs"], chainID)
	if errors.Is(outputChainErr, errOrderForDifferentChain) {
		return nil, outputChainErr
	}
	inputSettler, err := parseAddress(event.InputSettler, "inputSettler")
	if err != nil {
		return nil, err
	}
	// Unknown settlers can use a different token identifier format. Classify them before
	// decoding the protocol payload so their inputs cannot trigger our ERC-20 format alerts.
	if inputSettler != cfg.InputSettler {
		return nil, orderFieldError("unsupported_settler", "inputSettler", "inputSettler does not match configured settler", errOrderUnsupported)
	}
	if outputChainErr != nil {
		return nil, outputChainErr
	}
	var dto lifiorder.SubmitOrderDtoOrder
	if err := json.Unmarshal(event.Order, &dto); err != nil {
		return nil, decodeOrderError(rawOrder, err)
	}
	parsed, err := parseStandardOrder(dto)
	if err != nil {
		return nil, err
	}
	if err := validateOrderTarget(parsed.order, cfg, chainID); err != nil {
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

func validateOrderOutputChains(raw json.RawMessage, chainID int64) error {
	var outputs []json.RawMessage
	if len(raw) != 0 {
		if err := json.Unmarshal(raw, &outputs); err != nil {
			return orderFieldError("lifi_decode_error", "order.outputs", "decode submit order dto: invalid JSON shape", err)
		}
	}
	for i, output := range outputs {
		if err := validateOrderChain(orderRawObject(output)["chainId"], fmt.Sprintf("order.outputs[%d].chainId", i), chainID); err != nil {
			return err
		}
	}
	return nil
}

func validateOrderChain(value json.RawMessage, field string, chainID int64) error {
	var raw string
	if len(value) != 0 {
		if err := json.Unmarshal(value, &raw); err != nil {
			return orderFieldError("lifi_decode_error", field, "decode submit order dto: invalid JSON shape", err)
		}
	}
	id, err := parseUint(raw, field)
	if err != nil {
		return err
	}
	if id.Cmp(big.NewInt(chainID)) != 0 {
		return orderFieldError("unsupported_chain", field, fmt.Sprintf("%s: configuredChainId %d", errOrderForDifferentChain, chainID), errOrderForDifferentChain)
	}
	return nil
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
	if len(dto.Inputs) > 1 {
		return nil, orderFieldError("unsupported_multiple_inputs", "order.inputs", "multiple inputs are not supported", errOrderUnsupported)
	}
	if len(dto.Inputs) != 1 {
		return nil, orderFieldError("lifi_input_count", "order.inputs", "expected 1 input", nil)
	}
	if len(dto.Outputs) > 1 {
		return nil, orderFieldError("unsupported_multiple_outputs", "order.outputs", "multiple outputs are not supported", errOrderUnsupported)
	}
	if len(dto.Outputs) != 1 {
		return nil, orderFieldError("lifi_output_count", "order.outputs", "expected 1 output", nil)
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
		return nil, orderFieldError("lifi_input_tuple", "order.inputs[0]", "expected [tokenId, amount]", nil)
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
		return nil, orderFieldError("lifi_nonpositive_amount", "order.inputs[0][1]", "must be positive", nil)
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
	if tokenID == ([32]byte{}) {
		// This identifier is unsupported here; its asset meaning depends on the wire format.
		return nil, orderFieldError("lifi_zero_output_identifier", "order.outputs[0].token", "zero output identifier", errOrderUnsupported)
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
		return nil, orderFieldError("lifi_nonpositive_amount", "order.outputs[0].amount", "must be positive", nil)
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
		return nil, orderFieldError("unsupported_callback_data", "order.outputs[0].callbackData", "non-empty output callbackData is not supported", errOrderUnsupported)
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
	order inputsettler.StandardOrder,
	cfg *Config,
	chainID int64,
) error {
	// Recheck the decoded IDs: generated JSON decoding accepts case-insensitive keys,
	// while the raw routing lookups above use exact keys.
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
	if order.InputOracle != cfg.OutputSettler {
		return orderFieldError("unsupported_settler", "order.inputOracle", fmt.Sprintf(
			"order.inputOracle %s does not match outputSettler %s",
			order.InputOracle.Hex(),
			cfg.OutputSettler.Hex(),
		), errOrderUnsupported)
	}
	wantSettler := addressIdentifier(cfg.OutputSettler)
	if order.Outputs[0].Oracle != wantSettler {
		if _, err := identifierAddress(order.Outputs[0].Oracle, "order.outputs[0].oracle"); err != nil {
			return err
		}
		return orderFieldError("unsupported_settler", "order.outputs[0].oracle", "order.outputs[0].oracle does not match outputSettler", errOrderUnsupported)
	}
	if order.Outputs[0].Settler != wantSettler {
		if _, err := identifierAddress(order.Outputs[0].Settler, "order.outputs[0].settler"); err != nil {
			return err
		}
		return orderFieldError("unsupported_settler", "order.outputs[0].settler", "order.outputs[0].settler does not match outputSettler", errOrderUnsupported)
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
		return common.Address{}, orderFieldError("lifi_invalid_address", field, "invalid address", nil)
	}
	addr := common.HexToAddress(raw)
	if addr == (common.Address{}) {
		return common.Address{}, orderFieldError("lifi_zero_address", field, "zero address", nil)
	}
	return addr, nil
}

func parseUint32(raw, field string) (uint32, error) {
	n, err := parseUint(raw, field)
	if err != nil {
		return 0, err
	}
	if !n.IsUint64() || n.Uint64() > math.MaxUint32 {
		return 0, orderFieldError("lifi_uint32_overflow", field, "overflows uint32", nil)
	}
	return uint32(n.Uint64()), nil
}

func parseTupleUint(raw any, field string) (*big.Int, error) {
	value, ok := raw.(string)
	if !ok {
		return nil, orderFieldError("lifi_integer_type", field, "expected decimal string", nil)
	}
	return parseUint(value, field)
}

func parseUint(raw, field string) (*big.Int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, orderFieldError("lifi_empty_integer", field, "empty integer", nil)
	}
	n, ok := new(big.Int).SetString(raw, 10)
	if !ok || n.Sign() < 0 {
		return nil, orderFieldError("lifi_invalid_integer", field, "invalid uint", nil)
	}
	return n, nil
}

func parseBytes32(raw, field string) ([32]byte, error) {
	b, err := decodeHexBytes(raw, field)
	if err != nil {
		return [32]byte{}, err
	}
	if len(b) != 32 {
		return [32]byte{}, orderFieldError("lifi_identifier_length", field, "expected 32 bytes", nil)
	}
	var out [32]byte
	copy(out[:], b)
	return out, nil
}

func decodeHexBytes(raw, field string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, orderFieldError("lifi_empty_hex", field, "empty hex", nil)
	}
	if !strings.HasPrefix(raw, "0x") && !strings.HasPrefix(raw, "0X") {
		raw = "0x" + raw
	}
	out, err := hexutil.Decode(raw)
	if err != nil {
		return nil, orderFieldError("lifi_invalid_hex", field, "invalid hex", err)
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
	if n.Sign() == 0 {
		// LI.FI escrow uses token ID zero for native input. Our ERC-20 path never fills it.
		return common.Address{}, orderFieldError("unsupported_native_input", field, "native input token is not supported by this ERC-20 execution path", errNativeInputUnsupported)
	}
	if n.Sign() < 0 || n.BitLen() > common.AddressLength*8 {
		return common.Address{}, orderFieldError("invalid_token_identifier", field, "not a clean address identifier", nil)
	}
	return common.BytesToAddress(n.Bytes()), nil
}

func addressIdentifier(addr common.Address) [32]byte {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return out
}

func identifierAddress(id [32]byte, field string) (common.Address, error) {
	addr := common.BytesToAddress(id[12:])
	if addr == (common.Address{}) {
		return common.Address{}, orderFieldError("lifi_zero_identifier", field, "zero address identifier", nil)
	}
	if addressIdentifier(addr) != id {
		return common.Address{}, orderFieldError("lifi_unclean_identifier", field, "not a clean address identifier", nil)
	}
	return addr, nil
}
