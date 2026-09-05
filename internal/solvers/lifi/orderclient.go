package lifi

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/lifiorder"
	"github.com/symbioticfi/vault-solver/internal/solvers/lifi/strategies/types"
)

const (
	orderRecoveryPageLimit int32 = 50
	orderRecoveryMaxOffset int32 = 1_000
	orderStatusSigned            = "Signed"
	orderStatusDelivered         = "Delivered"
)

type orderClient struct {
	api    *lifiorder.APIClient
	apiKey string
	chain  string
}

func newOrderClient(baseURL, apiKey string, timeout time.Duration, chainID int64) *orderClient {
	cfg := lifiorder.NewConfiguration()
	cfg.Servers = lifiorder.ServerConfigurations{{URL: strings.TrimRight(baseURL, "/")}}
	cfg.HTTPClient = &http.Client{Timeout: timeout}
	return &orderClient{api: lifiorder.NewAPIClient(cfg), apiKey: apiKey, chain: strconv.FormatInt(chainID, 10)}
}

func (c *orderClient) withAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, lifiorder.ContextAPIKeys, map[string]lifiorder.APIKey{
		"api-key": {Key: c.apiKey},
	})
}

func (c *orderClient) validateExecutorRegistration(ctx context.Context, executor common.Address) error {
	identities, httpResp, err := c.api.SolverAPIAPI.
		SolverApiV0ControllerGetSolverIdentities(c.withAuth(ctx)).
		Execute()
	closeResp(httpResp)
	if err != nil {
		return apiErr("get solver identities", httpResp, err)
	}
	if identities != nil {
		for _, identity := range identities.Data {
			if strings.EqualFold(identity.Address, executor.Hex()) {
				return nil
			}
		}
	}
	return errors.Errorf("lifi order server: executor %s is not registered for this API key", executor.Hex())
}

func (c *orderClient) replaceSupportedContracts(
	ctx context.Context, dto lifiorder.PutSupportedContractsDto,
) error {
	_, httpResp, err := c.api.SolverAPIV1API.
		SupportedContractsControllerReplaceSupportedContracts(c.withAuth(ctx)).
		PutSupportedContractsDto(dto).
		Execute()
	closeResp(httpResp)
	if err != nil {
		return apiErr("put supported contracts", httpResp, err)
	}
	return nil
}

func (c *orderClient) ensureSupportedContracts(
	ctx context.Context, chainID int64, inputSettler, outputSettler common.Address,
) error {
	chain := chainRef(chainID)
	current, httpResp, err := c.api.SolverAPIV1API.
		SupportedContractsControllerGetSupportedContracts(c.withAuth(ctx)).
		Execute()
	closeResp(httpResp)
	if err != nil {
		return apiErr("get supported contracts", httpResp, err)
	}
	// The client tolerates a dropped field by zero-valuing it, and PUT replaces the whole registered
	// set, so an incomplete snapshot must not be merged or the missing kinds would be deregistered.
	if current == nil || current.Data.InputSettler == nil || current.Data.OutputSettler == nil {
		return errors.New("get supported contracts: incomplete snapshot; refusing to replace the registered set")
	}
	if supportsConfiguredContracts(current.Data, chain, inputSettler, outputSettler) {
		return nil
	}
	return c.replaceSupportedContracts(ctx, supportedContractsDTO(current.Data, chain, inputSettler, outputSettler))
}

func chainRef(chainID int64) string {
	return "eip155:" + strconv.FormatInt(chainID, 10)
}

func supportedContractsDTO(
	current lifiorder.ContractsByKindDto,
	chain string,
	inputSettler, outputSettler common.Address,
) lifiorder.PutSupportedContractsDto {
	dto := lifiorder.PutSupportedContractsDto{
		InputSettler:  supportedContractEntries(current.InputSettler),
		OutputSettler: supportedContractEntries(current.OutputSettler),
	}
	dto.InputSettler = appendSupportedContract(dto.InputSettler, chain, inputSettler)
	dto.OutputSettler = appendSupportedContract(dto.OutputSettler, chain, outputSettler)
	return dto
}

func supportedContractEntries(items []lifiorder.ChainAddressDto) []lifiorder.PutSupportedContractsDtoOracleInner {
	if len(items) == 0 {
		return nil
	}
	out := make([]lifiorder.PutSupportedContractsDtoOracleInner, len(items))
	for i, item := range items {
		out[i] = lifiorder.PutSupportedContractsDtoOracleInner(item)
	}
	return out
}

func appendSupportedContract(
	items []lifiorder.PutSupportedContractsDtoOracleInner,
	chain string,
	address common.Address,
) []lifiorder.PutSupportedContractsDtoOracleInner {
	for _, item := range items {
		if item.Chain == chain && strings.EqualFold(item.Address, address.Hex()) {
			return items
		}
	}
	return append(items, lifiorder.PutSupportedContractsDtoOracleInner{Chain: chain, Address: address.Hex()})
}

func supportsConfiguredContracts(
	contracts lifiorder.ContractsByKindDto,
	chain string,
	inputSettler, outputSettler common.Address,
) bool {
	return hasChainAddress(contracts.InputSettler, chain, inputSettler) &&
		hasChainAddress(contracts.OutputSettler, chain, outputSettler)
}

func hasChainAddress(items []lifiorder.ChainAddressDto, chain string, address common.Address) bool {
	for _, item := range items {
		if item.Chain == chain && strings.EqualFold(item.Address, address.Hex()) {
			return true
		}
	}
	return false
}

func (c *orderClient) listRecoverableOrders(
	ctx context.Context,
	executor common.Address,
) ([]json.RawMessage, error) {
	statuses := [...]string{orderStatusSigned, orderStatusDelivered}
	var orders []json.RawMessage
	for _, status := range statuses {
		listed, err := c.listRecoverableOrdersByStatus(ctx, executor, status)
		if err != nil {
			return nil, err
		}
		orders = append(orders, listed...)
	}
	return orders, nil
}

func (c *orderClient) listRecoverableOrdersByStatus(
	ctx context.Context,
	executor common.Address,
	status string,
) ([]json.RawMessage, error) {
	var orders []json.RawMessage
	for offset := int32(0); ; {
		// exclusiveFor scopes quote ownership to this solver; it is independent from
		// the output context's optional on-chain exclusivity window.
		response, httpResp, err := c.api.BridgeAPIAPI.
			OrdersControllerGetOrders(c.withAuth(ctx)).
			Limit(orderRecoveryPageLimit).
			Offset(offset).
			Status(status).
			ExclusiveFor(executor.Hex()).
			OriginChainId(c.chain).
			DestinationChainId(c.chain).
			Execute()
		closeResp(httpResp)
		if err != nil {
			return nil, apiErr("get "+status+" orders", httpResp, err)
		}
		if response == nil {
			return nil, errors.Errorf("lifi order server: get %s orders: empty response", status)
		}
		if len(response.Data) > int(orderRecoveryPageLimit) {
			return nil, errors.Errorf(
				"lifi order server: get %s orders: page has %d items, maximum is %d",
				status,
				len(response.Data),
				orderRecoveryPageLimit,
			)
		}
		for i := range response.Data {
			raw, marshalErr := json.Marshal(response.Data[i])
			if marshalErr != nil {
				return nil, errors.Errorf("lifi order server: encode %s order %d: %w", status, i, marshalErr)
			}
			orders = append(orders, raw)
		}

		pageSize := int32(len(response.Data))
		nextOffset := offset + pageSize
		if response.Meta.Total > 0 {
			if float32(nextOffset) >= response.Meta.Total {
				return orders, nil
			}
			if pageSize == 0 {
				return nil, errors.Errorf(
					"lifi order server: get %s orders: empty page at offset %d before total %v",
					status,
					offset,
					response.Meta.Total,
				)
			}
		} else if pageSize < orderRecoveryPageLimit {
			return orders, nil
		}
		if nextOffset > orderRecoveryMaxOffset {
			return nil, errors.Errorf(
				"lifi order server: get %s orders: pagination requires offset %d, maximum is %d (reported total %v)",
				status,
				nextOffset,
				orderRecoveryMaxOffset,
				response.Meta.Total,
			)
		}
		offset = nextOffset
	}
}

func (c *orderClient) submitQuotes(ctx context.Context, quotes []types.Quote) error {
	dtoQuotes := make([]lifiorder.SubmitQuotesDtoQuotesInner, 0, len(quotes))
	expectedRanges := 0
	for i, quote := range quotes {
		dto, err := submitQuoteDTO(c.chain, quote, i)
		if err != nil {
			return err
		}
		dtoQuotes = append(dtoQuotes, dto)
		expectedRanges += len(dto.Ranges)
	}

	response, httpResp, err := c.api.SolverAPIAPI.
		QuoteSubmissionControllerSubmitQuotes(c.withAuth(ctx)).
		SubmitQuotesDto(lifiorder.SubmitQuotesDto{Quotes: dtoQuotes}).
		Execute()
	closeResp(httpResp)
	if err != nil {
		return apiErr("submit quotes", httpResp, err)
	}
	if response == nil {
		return errors.New("lifi order server: submit quotes: empty response")
	}
	if response.QuotesAdded != float32(expectedRanges) {
		return errors.Errorf(
			"lifi order server: submit quotes: quotesAdded %v, want %d",
			response.QuotesAdded,
			expectedRanges,
		)
	}
	return nil
}

func submitQuoteDTO(chain string, quote types.Quote, index int) (lifiorder.SubmitQuotesDtoQuotesInner, error) {
	field := "quotes[" + strconv.Itoa(index) + "]"
	expiry, err := int32Checked(quote.Expiry, field+".expiry")
	if err != nil {
		return lifiorder.SubmitQuotesDtoQuotesInner{}, err
	}
	fromDecimals, err := int32Checked(int64(quote.FromDecimals), field+".fromDecimals")
	if err != nil {
		return lifiorder.SubmitQuotesDtoQuotesInner{}, err
	}
	toDecimals, err := int32Checked(int64(quote.ToDecimals), field+".toDecimals")
	if err != nil {
		return lifiorder.SubmitQuotesDtoQuotesInner{}, err
	}
	ranges := make([]lifiorder.SubmitQuotesDtoQuotesInnerRangesInner, 0, len(quote.Ranges))
	for i, quoteRange := range quote.Ranges {
		if quoteRange.MinAmount == nil || quoteRange.MaxAmount == nil || quoteRange.Quote == "" {
			return lifiorder.SubmitQuotesDtoQuotesInner{}, errors.Errorf("%s.ranges[%d]: incomplete range", field, i)
		}
		ranges = append(ranges, lifiorder.SubmitQuotesDtoQuotesInnerRangesInner{
			MinAmount: quoteRange.MinAmount.String(),
			MaxAmount: quoteRange.MaxAmount.String(),
			Quote:     quoteRange.Quote,
		})
	}
	dto := lifiorder.SubmitQuotesDtoQuotesInner{
		FromChain: chain, ToChain: chain,
		FromAsset: quote.FromAsset.Hex(), ToAsset: quote.ToAsset.Hex(),
		FromDecimals: fromDecimals, ToDecimals: toDecimals,
		Ranges: ranges, Expiry: expiry,
	}
	if quote.ExclusiveFor != (common.Address{}) {
		exclusiveFor := quote.ExclusiveFor.Hex()
		dto.ExclusiveFor = &exclusiveFor
	}
	return dto, nil
}

func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func apiErr(what string, resp *http.Response, err error) error {
	var genErr *lifiorder.GenericOpenAPIError
	if errors.As(err, &genErr) {
		if body := strings.TrimSpace(string(genErr.Body())); body != "" {
			return errors.Errorf("lifi order server: %s: %s: %s: %w", what, statusOf(resp), body, err)
		}
	}
	return errors.Errorf("lifi order server: %s: %s: %w", what, statusOf(resp), err)
}

func statusOf(resp *http.Response) string {
	if resp == nil {
		return "no response"
	}
	return resp.Status
}

func int32Checked(v int64, field string) (int32, error) {
	if v < math.MinInt32 || v > math.MaxInt32 {
		return 0, errors.Errorf("%s: %d overflows int32", field, v)
	}
	return int32(v), nil
}
