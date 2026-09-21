package chain

import (
	"context"
	"math/big"

	"github.com/go-errors/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// callTracing describes how one dialed endpoint's JSON-RPC calls are traced. Only a non-HTTP
// endpoint sets transport: an HTTP(S) endpoint already gets one span per request from
// fallbackTransport, and an empty transport turns every method below into a plain passthrough so
// that path's spans stay exactly as they were.
type callTracing struct {
	role      string
	transport string
}

// start opens the client span for one JSON-RPC method. The returned func ends it and is meant to be
// called once, from a defer over the caller's named error return.
func (t callTracing) start(ctx context.Context, method string) (context.Context, func(error)) {
	if t.transport == "" {
		return ctx, func(error) {}
	}
	ctx, end := rpcTracer.StartKind(ctx, method, trace.SpanKindClient,
		attribute.String("rpc.system", "jsonrpc"),
		attribute.String("rpc.method", method),
		attribute.String("chain.rpc.role", t.role),
		attribute.String("chain.rpc.transport", t.transport),
	)
	return ctx, func(err error) { end(classifyRPCSpanError(ctx, err)) }
}

// The methods below shadow the promoted ethclient ones so a websocket or IPC endpoint still produces
// one client span per JSON-RPC call, named and attributed like the HTTP transport's spans. Only the
// methods this repo actually calls through the client are shadowed; a new call site needs a new
// shadow here or it goes untraced on those transports. Multicall is deliberately absent: it reaches
// the chain through CallContract, which is where its span belongs.

// CallContract executes an eth_call against the read endpoint.
func (c *Client) CallContract(
	ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int,
) (_ []byte, err error) {
	ctx, end := c.readCalls.start(ctx, rpcMethodCall)
	defer func() { end(err) }()
	return c.Client.CallContract(ctx, msg, blockNumber)
}

// HeaderByNumber reads a block header by number through the read endpoint.
func (c *Client) HeaderByNumber(ctx context.Context, number *big.Int) (_ *types.Header, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_getBlockByNumber")
	defer func() { end(err) }()
	return c.Client.HeaderByNumber(ctx, number)
}

// HeaderByHash reads a block header by hash through the read endpoint.
func (c *Client) HeaderByHash(ctx context.Context, hash common.Hash) (_ *types.Header, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_getBlockByHash")
	defer func() { end(err) }()
	return c.Client.HeaderByHash(ctx, hash)
}

// FeeHistory reads recent base fees and reward percentiles through the read endpoint.
func (c *Client) FeeHistory(
	ctx context.Context, blockCount uint64, lastBlock *big.Int, rewardPercentiles []float64,
) (_ *ethereum.FeeHistory, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_feeHistory")
	defer func() { end(err) }()
	return c.Client.FeeHistory(ctx, blockCount, lastBlock, rewardPercentiles)
}

// SuggestGasTipCap reads the node's suggested priority fee through the read endpoint.
func (c *Client) SuggestGasTipCap(ctx context.Context) (_ *big.Int, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_maxPriorityFeePerGas")
	defer func() { end(err) }()
	return c.Client.SuggestGasTipCap(ctx)
}

// EstimateGas estimates a call's gas through the read endpoint.
func (c *Client) EstimateGas(ctx context.Context, msg ethereum.CallMsg) (_ uint64, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_estimateGas")
	defer func() { end(err) }()
	return c.Client.EstimateGas(ctx, msg)
}

// TransactionReceipt reads a receipt through the read endpoint.
func (c *Client) TransactionReceipt(ctx context.Context, txHash common.Hash) (_ *types.Receipt, err error) {
	ctx, end := c.readCalls.start(ctx, rpcMethodGetTransactionReceipt)
	defer func() { end(err) }()
	return c.Client.TransactionReceipt(ctx, txHash)
}

// BalanceAt reads an account balance through the read endpoint.
func (c *Client) BalanceAt(
	ctx context.Context, account common.Address, blockNumber *big.Int,
) (_ *big.Int, err error) {
	ctx, end := c.readCalls.start(ctx, rpcMethodGetBalance)
	defer func() { end(err) }()
	return c.Client.BalanceAt(ctx, account, blockNumber)
}

// CodeAt reads account code through the read endpoint.
func (c *Client) CodeAt(
	ctx context.Context, account common.Address, blockNumber *big.Int,
) (_ []byte, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_getCode")
	defer func() { end(err) }()
	return c.Client.CodeAt(ctx, account, blockNumber)
}

// BlockNumber reads the head block number through the read endpoint.
func (c *Client) BlockNumber(ctx context.Context) (_ uint64, err error) {
	ctx, end := c.readCalls.start(ctx, "eth_blockNumber")
	defer func() { end(err) }()
	return c.Client.BlockNumber(ctx)
}

// SendTransaction broadcasts a signed transaction through the write client. It overrides the
// promoted ethclient method.
func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) (err error) {
	ctx, end := c.writeCalls.start(ctx, rpcMethodSendRawTransaction)
	defer func() { end(err) }()
	return c.writeClient.SendTransaction(ctx, tx)
}

// SendCancellationTransaction broadcasts a same-nonce self-cancellation through the cancellation
// client, or the ordinary write client when no cancellation endpoint is configured.
func (c *Client) SendCancellationTransaction(ctx context.Context, tx *types.Transaction) (err error) {
	ctx, end := c.cancelCalls.start(ctx, rpcMethodSendRawTransaction)
	defer func() { end(err) }()
	return c.cancelClient.SendTransaction(ctx, tx)
}

// NonceAt reads the mined nonce through the write client so startup compares one endpoint's mined
// and pending views instead of failing on harmless head skew between independent RPC nodes.
func (c *Client) NonceAt(
	ctx context.Context, account common.Address, blockNumber *big.Int,
) (_ uint64, err error) {
	ctx, end := c.writeCalls.start(ctx, rpcMethodGetTransactionCount)
	defer func() { end(err) }()
	return c.writeClient.NonceAt(ctx, account, blockNumber)
}

// PendingNonceAt reads the pending nonce through the write client so a private write endpoint can
// report transactions that are not visible to the primary RPC. It overrides the promoted ethclient
// method. When no separate write endpoint is configured, it targets the primary endpoint.
func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (_ uint64, err error) {
	ctx, end := c.writeCalls.start(ctx, rpcMethodGetTransactionCount)
	defer func() { end(err) }()
	return c.writeClient.PendingNonceAt(ctx, account)
}

// TransactionSenderBalanceAt prefers the write endpoint for sender telemetry, then falls back to the
// ordinary read client when a distinct submission endpoint rejects or cannot serve eth_getBalance.
func (c *Client) TransactionSenderBalanceAt(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (*big.Int, error) {
	balance, writeErr := c.writeBalanceAt(ctx, account, blockNumber)
	if writeErr == nil || c.writeClient == c.Client {
		return balance, writeErr
	}
	balance, readErr := c.BalanceAt(ctx, account, blockNumber)
	if readErr == nil {
		return balance, nil
	}
	return nil, errors.Errorf(
		"chain: transaction sender balance: %w",
		errors.Join(
			errors.Errorf("write endpoint: %w", writeErr),
			errors.Errorf("read endpoints: %w", readErr),
		),
	)
}

func (c *Client) writeBalanceAt(
	ctx context.Context, account common.Address, blockNumber *big.Int,
) (_ *big.Int, err error) {
	ctx, end := c.writeCalls.start(ctx, rpcMethodGetBalance)
	defer func() { end(err) }()
	return c.writeClient.BalanceAt(ctx, account, blockNumber)
}
