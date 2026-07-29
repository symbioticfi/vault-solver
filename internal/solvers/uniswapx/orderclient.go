package uniswapx

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/uniswapxservice"
)

const (
	maxOrderResponseBytes = 4 << 20
	orderPageLimit        = 1000
	maxOrderPages         = 10
	maxOrderHashBatch     = 50
	minOrderRequestGap    = time.Second / 6
	betaRFQHeaderValue    = "true"
)

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
	return c.orders(ctx, chainID, filler, orderStatusOpen, time.Time{})
}

func (c *orderClient) recentOrders(
	ctx context.Context,
	chainID int64,
	filler common.Address,
	createdAfter time.Time,
) ([]orderEntry, error) {
	if filler == (common.Address{}) {
		return nil, errors.New("GET /orders history: zero filler")
	}
	if createdAfter.IsZero() {
		return nil, errors.New("GET /orders history: zero created-after time")
	}
	return c.orders(ctx, chainID, &filler, "", createdAfter)
}

func (c *orderClient) orders(
	ctx context.Context,
	chainID int64,
	filler *common.Address,
	status string,
	createdAfter time.Time,
) ([]orderEntry, error) {
	var orders []orderEntry
	var cursor string
	seenCursors := make(map[string]bool)
	for range maxOrderPages {
		page, err := c.orderPage(ctx, chainID, filler, status, createdAfter, cursor)
		if err != nil {
			return orders, err
		}
		orders = append(orders, page.Orders...)
		if page.Cursor == "" {
			return orders, nil
		}
		if seenCursors[page.Cursor] {
			return orders, errors.New("orders response repeated its cursor")
		}
		seenCursors[page.Cursor] = true
		cursor = page.Cursor
	}
	return orders, errors.Errorf("orders response exceeds %d pages", maxOrderPages)
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
	for start := 0; start < len(hashes); start += maxOrderHashBatch {
		end := min(start+maxOrderHashBatch, len(hashes))
		if err := c.fetchOrderHashBatch(ctx, chainID, hashes[start:end], terminals); err != nil {
			return nil, err
		}
	}
	for hash := range requested {
		if _, ok := terminals[hash]; !ok {
			return nil, errors.Errorf("GET /orders by hash: missing order %s", hash.Hex())
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
	if err := c.waitForRequestSlot(ctx); err != nil {
		return errors.Errorf("wait for orders rate limit: %w", err)
	}
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
	response, httpResponse, err := request.Execute()
	if httpResponse != nil && httpResponse.Body != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		return errors.Errorf("GET /orders by hash: %w", err)
	}
	if response == nil {
		return errors.New("GET /orders by hash: empty response")
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

func (c *orderClient) orderPage(
	ctx context.Context,
	chainID int64,
	filler *common.Address,
	status string,
	createdAfter time.Time,
	cursor string,
) (orderPage, error) {
	if err := c.waitForRequestSlot(ctx); err != nil {
		return orderPage{}, errors.Errorf("wait for orders rate limit: %w", err)
	}
	response, err := c.executeOrderRequest(ctx, chainID, filler, status, createdAfter, cursor)
	if err != nil {
		return orderPage{}, err
	}
	if response == nil {
		return orderPage{}, errors.New("GET /orders: empty response")
	}
	if len(response.Orders) > orderPageLimit {
		return orderPage{}, errors.Errorf(
			"GET /orders: response contains %d orders, max %d",
			len(response.Orders),
			orderPageLimit,
		)
	}
	orders := make([]orderEntry, 0, len(response.Orders))
	for i := range response.Orders {
		order := response.Orders[i].DutchV2OrderEntity
		if order == nil {
			return orderPage{}, errors.Errorf("GET /orders: order %d is not Dutch_V2", i)
		}
		entry, err := orderEntryFromAPI(order)
		if err != nil {
			return orderPage{}, errors.Errorf("GET /orders: order %d: %w", i, err)
		}
		orders = append(orders, entry)
	}
	return orderPage{Orders: orders, Cursor: response.GetCursor()}, nil
}

func (c *orderClient) executeOrderRequest(
	ctx context.Context,
	chainID int64,
	filler *common.Address,
	status string,
	createdAfter time.Time,
	cursor string,
) (*uniswapxservice.GetOrdersResponse, error) {
	request := c.client.OrdersAPI.OrdersGet(ctx).
		ChainId(uniswapxservice.ChainId(chainID)).
		Limit(orderPageLimit).
		SortKey(uniswapxservice.CREATED_AT).
		Desc(true).
		OrderType(uniswapxservice.DUTCH_V2)
	if status != "" {
		request = request.OrderStatus(uniswapxservice.OrderStatus(status))
	}
	if createdAfter.IsZero() {
		request = request.Sort("gt(0)")
	} else {
		request = request.Sort("gt(" + strconv.FormatInt(createdAfter.Unix(), 10) + ")")
	}
	if filler != nil {
		request = request.Filler(filler.Hex())
	}
	if cursor != "" {
		request = request.Cursor(cursor)
	}
	response, httpResponse, err := request.Execute()
	if httpResponse != nil && httpResponse.Body != nil {
		defer httpResponse.Body.Close()
	}
	if err != nil {
		return nil, errors.Errorf("GET /orders: %w", err)
	}
	return response, nil
}

func orderTerminalFromAPI(
	order *uniswapxservice.DutchV2OrderEntity,
	chainID int64,
) (common.Hash, orderTerminal, error) {
	if order.Type != orderTypeDutchV2 {
		return common.Hash{}, orderTerminal{}, errors.Errorf("unexpected order type %q", order.Type)
	}
	if int64(order.ChainId) != chainID {
		return common.Hash{}, orderTerminal{}, errors.Errorf(
			"order chain id %d does not match %d",
			int64(order.ChainId),
			chainID,
		)
	}
	orderHash, err := decodeHash(order.OrderHash)
	if err != nil || orderHash == (common.Hash{}) {
		return common.Hash{}, orderTerminal{}, errors.Errorf("invalid order hash %q", order.OrderHash)
	}
	if !order.OrderStatus.IsValid() {
		return common.Hash{}, orderTerminal{}, errors.Errorf("invalid order status %q", order.OrderStatus)
	}

	terminal := orderTerminal{Status: string(order.OrderStatus)}
	txHashValue, hasTxHash := order.GetTxHashOk()
	if hasTxHash {
		txHash, decodeErr := decodeHash(*txHashValue)
		if decodeErr != nil || txHash == (common.Hash{}) {
			return common.Hash{}, orderTerminal{}, errors.Errorf("invalid transaction hash %q", *txHashValue)
		}
		terminal.TxHash = txHash
	}
	if terminal.Status == orderStatusFilled {
		if !hasTxHash {
			return common.Hash{}, orderTerminal{}, errors.New("filled order has no transaction hash")
		}
	} else if hasTxHash {
		return common.Hash{}, orderTerminal{}, errors.Errorf(
			"status %q unexpectedly has transaction hash",
			terminal.Status,
		)
	}
	return orderHash, terminal, nil
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
		QuoteID: order.GetQuoteId(),
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
	response.Body = &limitedResponseBody{
		Reader: &errorLimitReader{reader: response.Body, remaining: t.limit},
		Closer: response.Body,
	}
	return response, nil
}

type limitedResponseBody struct {
	io.Reader
	io.Closer
}

type errorLimitReader struct {
	reader    io.Reader
	remaining int64
}

func (r *errorLimitReader) Read(data []byte) (int, error) {
	if r.remaining <= 0 {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, errors.New("order response exceeds size limit")
		}
		return 0, err
	}
	if int64(len(data)) > r.remaining {
		data = data[:r.remaining]
	}
	n, err := r.reader.Read(data)
	r.remaining -= int64(n)
	return n, err
}

func (c *orderClient) waitForRequestSlot(ctx context.Context) error {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	if c.requestGap <= 0 {
		return nil
	}
	delay := time.Until(c.lastRequest.Add(c.requestGap))
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	c.lastRequest = time.Now()
	return nil
}
