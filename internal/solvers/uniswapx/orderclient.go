package uniswapx

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/uniswapxservice"
	"github.com/symbioticfi/vault-solver/internal/httpclient"
)

const (
	maxOrderResponseBytes = 4 << 20
	orderPageLimit        = 50
	maxOrderHashBatch     = 50
	minOrderRequestGap    = time.Second / 6
	betaRFQHeaderValue    = "true"
)

var errOrderSnapshotTruncated = errors.New("orders snapshot may be truncated")

type orderClient struct {
	client *uniswapxservice.APIClient

	requestMu   sync.Mutex
	lastRequest time.Time
	requestGap  time.Duration
}

func newOrderClient(cfg OrderServerConfig, apiKey string) *orderClient {
	generatedConfig := uniswapxservice.NewConfiguration()
	generatedConfig.Servers = uniswapxservice.ServerConfigurations{{URL: strings.TrimRight(cfg.BaseURL, "/")}}
	generatedConfig.DefaultHeader["x-api-key"] = apiKey
	if cfg.Beta {
		generatedConfig.DefaultHeader["x-beta-rfq"] = betaRFQHeaderValue
	}
	generatedConfig.HTTPClient = &http.Client{
		Timeout:   cfg.HTTPTimeout,
		Transport: responseLimitTransport{next: http.DefaultTransport, limit: maxOrderResponseBytes},
	}
	return &orderClient{
		client:     uniswapxservice.NewAPIClient(generatedConfig),
		requestGap: minOrderRequestGap,
	}
}

func (c *orderClient) openOrders(ctx context.Context, chainID int64, filler *common.Address) ([]orderEntry, error) {
	return c.orders(ctx, chainID, filler, orderStatusOpen)
}

func (c *orderClient) recentOrders(ctx context.Context, chainID int64, filler common.Address, createdAfter time.Time) ([]orderEntry, error) {
	if filler == (common.Address{}) || createdAfter.IsZero() {
		return nil, errors.New("GET /orders history: filler and created-after time are required")
	}
	snapshot, fetchErr := c.orders(ctx, chainID, &filler, "")
	selected := make([]orderEntry, 0, len(snapshot))
	covered := false
	for index, entry := range snapshot {
		if entry.CreatedAt <= 0 {
			return selected, errors.Join(fetchErr, errors.Errorf("GET /orders history: order %d has no valid createdAt", index))
		}
		if index > 0 && entry.CreatedAt > snapshot[index-1].CreatedAt {
			return selected, errors.Join(fetchErr, errors.New("GET /orders history: response is not newest-first"))
		}
		if entry.CreatedAt <= createdAfter.Unix() {
			covered = true
		} else {
			selected = append(selected, entry)
		}
	}
	// A truncated suffix is harmless only after validating the whole returned page
	// and proving it reaches beyond the requested history window.
	if covered && errors.Is(fetchErr, errOrderSnapshotTruncated) {
		fetchErr = nil
	}
	return selected, fetchErr
}

func (c *orderClient) orders(
	ctx context.Context,
	chainID int64,
	filler *common.Address,
	status string,
) ([]orderEntry, error) {
	response, err := c.executeOrderRequest(ctx, chainID, filler, status)
	if err != nil {
		return nil, err
	}
	if len(response.Orders) > orderPageLimit {
		return nil, errors.Errorf(
			"GET /orders: response contains %d orders, max %d",
			len(response.Orders),
			orderPageLimit,
		)
	}
	orders := make([]orderEntry, 0, len(response.Orders))
	for i := range response.Orders {
		order := response.Orders[i].DutchV2OrderEntity
		if order == nil {
			return nil, errors.Errorf("GET /orders: order %d is not Dutch_V2", i)
		}
		entry, convertErr := orderEntryFromAPI(order)
		if convertErr != nil {
			return nil, errors.Errorf("GET /orders: order %d: %w", i, convertErr)
		}
		orders = append(orders, entry)
	}
	if response.GetCursor() != "" {
		return orders, errors.New("GET /orders: unexpected cursor from non-paginated endpoint")
	}
	if len(orders) == orderPageLimit {
		return orders, errors.Errorf("%w: response reached the %d-order server limit", errOrderSnapshotTruncated, orderPageLimit)
	}
	return orders, nil
}

func (c *orderClient) ordersByHash(
	ctx context.Context,
	chainID int64,
	hashes []common.Hash,
) (map[common.Hash]orderTerminal, error) {
	requested := make(map[common.Hash]struct{}, len(hashes))
	for _, hash := range hashes {
		if hash == (common.Hash{}) {
			return nil, errors.New("GET /orders by hash: zero order hash")
		}
		if _, duplicate := requested[hash]; duplicate {
			return nil, errors.Errorf("GET /orders by hash: duplicate requested hash %s", hash.Hex())
		}
		requested[hash] = struct{}{}
	}

	terminals := make(map[common.Hash]orderTerminal, len(hashes))
	for batch := range slices.Chunk(hashes, maxOrderHashBatch) {
		if err := c.fetchOrderHashBatch(ctx, chainID, batch, terminals); err != nil {
			return nil, err
		}
	}
	return terminals, nil
}

func (c *orderClient) fetchOrderHashBatch(
	ctx context.Context,
	chainID int64,
	hashes []common.Hash,
	terminals map[common.Hash]orderTerminal,
) error {
	hashValues := make([]string, len(hashes))
	batch := make(map[common.Hash]struct{}, len(hashes))
	for i, hash := range hashes {
		hashValues[i] = hash.Hex()
		batch[hash] = struct{}{}
	}
	request := c.client.OrdersAPI.OrdersGet(ctx).
		ChainId(uniswapxservice.ChainId(chainID)).
		Limit(float32(len(hashes))).
		OrderHashes(strings.Join(hashValues, ",")).
		OrderType(uniswapxservice.DUTCH_V2)
	response, err := c.execute(ctx, request, "GET /orders by hash")
	if err != nil {
		return err
	}
	if response.GetCursor() != "" {
		return errors.New("GET /orders by hash: unexpected paginated response")
	}
	for i := range response.Orders {
		order := response.Orders[i].DutchV2OrderEntity
		if order == nil {
			return errors.Errorf("GET /orders by hash: order %d is not Dutch_V2", i)
		}
		hash, terminal, convertErr := orderTerminalFromAPI(order, chainID)
		if convertErr != nil {
			return errors.Errorf("GET /orders by hash: order %d: %w", i, convertErr)
		}
		if _, ok := batch[hash]; !ok {
			return errors.Errorf("GET /orders by hash: unexpected order %s", hash.Hex())
		}
		if _, duplicate := terminals[hash]; duplicate {
			return errors.Errorf("GET /orders by hash: duplicate order %s", hash.Hex())
		}
		terminals[hash] = terminal
	}
	for hash := range batch {
		if _, ok := terminals[hash]; !ok {
			return errors.Errorf("GET /orders by hash: missing order %s", hash.Hex())
		}
	}
	return nil
}

func (c *orderClient) executeOrderRequest(
	ctx context.Context,
	chainID int64,
	filler *common.Address,
	status string,
) (*uniswapxservice.GetOrdersResponse, error) {
	request := c.client.OrdersAPI.OrdersGet(ctx).
		ChainId(uniswapxservice.ChainId(chainID)).
		Limit(orderPageLimit).
		OrderType(uniswapxservice.DUTCH_V2)
	if status != "" {
		request = request.OrderStatus(uniswapxservice.OrderStatus(status))
	}
	if filler != nil {
		request = request.Filler(filler.Hex())
	}
	return c.execute(ctx, request, "GET /orders")
}

// Every request, including status reconciliation, passes the same rate and response boundary.
func (c *orderClient) execute(ctx context.Context, request uniswapxservice.ApiOrdersGetRequest, operation string) (*uniswapxservice.GetOrdersResponse, error) {
	if err := c.waitForRequestSlot(ctx); err != nil {
		return nil, errors.Errorf("wait for orders rate limit: %w", err)
	}
	response, err := httpclient.Execute(operation, request.Execute)

	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.Errorf("%s: empty response", operation)
	}
	return response, nil
}

func orderTerminalFromAPI(order *uniswapxservice.DutchV2OrderEntity, chainID int64) (hash common.Hash, terminal orderTerminal, err error) {
	// A status response must bind the same protocol and chain before it can retire local work.
	if order == nil {
		return hash, terminal, errors.New("order is missing")
	}
	switch {
	case order.Type != orderTypeDutchV2:
		return hash, terminal, errors.Errorf("unexpected order type %q", order.Type)
	case int64(order.ChainId) != chainID:
		return hash, terminal, errors.Errorf("order chain id %d does not match %d", int64(order.ChainId), chainID)
	case !order.OrderStatus.IsValid():
		return hash, terminal, errors.Errorf("invalid order status %q", order.OrderStatus)
	}
	hash, err = decodeHash(order.OrderHash)
	if err != nil || hash == (common.Hash{}) {
		return common.Hash{}, orderTerminal{}, errors.Errorf("invalid order hash %q", order.OrderHash)
	}
	terminal.Status = string(order.OrderStatus)
	transaction, present := order.GetTxHashOk()
	if present {
		terminal.TxHash, err = decodeHash(*transaction)
		if err != nil || terminal.TxHash == (common.Hash{}) {
			return common.Hash{}, orderTerminal{}, errors.Errorf("invalid transaction hash %q", *transaction)
		}
	}
	if present != (terminal.Status == orderStatusFilled) {
		if !present {
			return common.Hash{}, orderTerminal{}, errors.New("filled order has no transaction hash")
		}
		return common.Hash{}, orderTerminal{}, errors.Errorf("status %q unexpectedly has transaction hash", terminal.Status)
	}
	return hash, terminal, nil
}

func decodeHash(value string) (common.Hash, error) {
	decoded, err := hexutil.Decode(value)
	if err != nil {
		return common.Hash{}, err
	}
	if len(decoded) != common.HashLength {
		return common.Hash{}, errors.Errorf("got %d bytes, want %d", len(decoded), common.HashLength)
	}
	return common.BytesToHash(decoded), nil
}

func orderEntryFromAPI(order *uniswapxservice.DutchV2OrderEntity) (orderEntry, error) {
	if order.Type != orderTypeDutchV2 {
		return orderEntry{}, errors.Errorf("unexpected order type %q", order.Type)
	}
	if order.Input == nil {
		return orderEntry{}, errors.New("input is missing")
	}
	outputs := make([]orderOutput, 0, len(order.Outputs))
	for i := range order.Outputs {
		output := &order.Outputs[i]
		outputs = append(outputs, orderOutput{
			Token: output.GetToken(), StartAmount: output.StartAmount,
			EndAmount: output.EndAmount, Recipient: output.Recipient,
		})
	}
	return orderEntry{
		Type: order.Type, EncodedOrder: order.EncodedOrder, Signature: order.Signature,
		OrderHash: order.OrderHash, OrderStatus: string(order.OrderStatus), ChainID: int64(order.ChainId),
		QuoteID: order.GetQuoteId(), CreatedAt: int64(order.GetCreatedAt()),
		Input: orderToken{
			Token: order.Input.Token, StartAmount: order.Input.GetStartAmount(), EndAmount: order.Input.GetEndAmount(),
		},
		Outputs: outputs,
	}, nil
}

type responseLimitTransport struct {
	next  http.RoundTripper
	limit int64
}

func (t responseLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(request)
	if err != nil || response == nil || response.Body == nil {
		return response, err
	}
	response.Body = &limitedResponseBody{ReadCloser: response.Body, remaining: t.limit}
	return response, nil
}

// The body owns both its byte budget and underlying Close operation.
type limitedResponseBody struct {
	io.ReadCloser

	remaining int64
}

func (r *limitedResponseBody) Read(data []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, errors.New("order response exceeds size limit")
		}
		return 0, err
	}
	if int64(len(data)) > r.remaining {
		data = data[:r.remaining]
	}
	n, err := r.ReadCloser.Read(data)
	r.remaining -= int64(n)
	return n, err
}

func (c *orderClient) waitForRequestSlot(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		c.requestMu.Lock()
		delay := time.Until(c.lastRequest.Add(c.requestGap))
		if delay <= 0 || c.requestGap <= 0 {
			c.lastRequest = time.Now()
			c.requestMu.Unlock()
			return nil
		}
		c.requestMu.Unlock()
		// Waiting callers hold no mutex, so each can cancel independently.
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
