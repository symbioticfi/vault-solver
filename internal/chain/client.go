// Package chain owns generic EVM connectivity and generated-binding based batched reads.
package chain

import (
	"context"
	"math/big"
	"net/http"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

// Client separates a fallback-capable read plane from the single-endpoint write/nonce plane.
// The embedded client intentionally exposes go-ethereum's read methods to integration gateways.
type Client struct {
	*ethclient.Client

	writes    *ethclient.Client
	chainID   *big.Int
	multicall common.Address
}

// Dial establishes the read plane, verifies its chain id, then establishes the isolated write
// plane. An explicit write endpoint must prove that it belongs to the same chain.
func Dial(
	ctx context.Context,
	readURLs []string,
	writeURL string,
	multicallAddress string,
	log logr.Logger,
) (*Client, error) {
	return dial(ctx, readURLs, writeURL, multicallAddress, nil, log)
}

// DialWithMetrics instruments bounded HTTP JSON-RPC calls without changing the chain API.
func DialWithMetrics(
	ctx context.Context,
	readURLs []string,
	writeURL string,
	multicallAddress string,
	metrics *RPCMetrics,
	log logr.Logger,
) (*Client, error) {
	return dial(ctx, readURLs, writeURL, multicallAddress, metrics, log)
}

func dial(
	ctx context.Context,
	readURLs []string,
	writeURL string,
	multicallAddress string,
	metrics *RPCMetrics,
	log logr.Logger,
) (*Client, error) {
	if len(readURLs) == 0 {
		return nil, errors.New("chain: no rpc url configured")
	}
	if !common.IsHexAddress(multicallAddress) {
		return nil, errors.Errorf("chain: invalid multicall address %q", multicallAddress)
	}

	writeEndpoint := writeURL
	if writeEndpoint == "" && len(readURLs) > 1 {
		writeEndpoint = readURLs[0]
	}
	readRole := rpcRoleRead
	if writeEndpoint == "" {
		readRole = rpcRoleShared
	}
	reads, err := dialRPC(ctx, readURLs, readRole, metrics, log)
	if err != nil {
		return nil, err
	}
	chainID, err := reads.ChainID(ctx)
	if err != nil {
		reads.Close()
		return nil, errors.Errorf("chain: get chain id: %w", err)
	}
	writes, err := dialWritePlane(ctx, readURLs, writeURL, reads, chainID, metrics, log)
	if err != nil {
		reads.Close()
		return nil, err
	}
	return &Client{
		Client:    reads,
		writes:    writes,
		chainID:   new(big.Int).Set(chainID),
		multicall: common.HexToAddress(multicallAddress),
	}, nil
}

func dialRPC(
	ctx context.Context,
	endpoints []string,
	role string,
	metrics *RPCMetrics,
	log logr.Logger,
) (*ethclient.Client, error) {
	if len(endpoints) == 1 && !isHTTPURL(endpoints[0]) {
		client, err := ethclient.DialContext(ctx, endpoints[0])
		if err != nil {
			return nil, errors.Errorf("chain: dial: %w", err)
		}
		return client, nil
	}
	parsed, err := parseHTTPEndpoints(endpoints)
	if err != nil {
		return nil, err
	}
	httpClient := &http.Client{Transport: &fallbackTransport{
		endpoints: parsed,
		base:      http.DefaultTransport,
		metrics:   metrics,
		role:      role,
		log:       log,
	}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	metrics.bindTransport(role, len(parsed))
	rpcClient, err := rpc.DialOptions(ctx, endpoints[0], rpc.WithHTTPClient(httpClient))
	if err != nil {
		return nil, errors.Errorf("chain: dial (fallback): %w", err)
	}
	return ethclient.NewClient(rpcClient), nil
}

func dialWritePlane(
	ctx context.Context,
	readURLs []string,
	explicitURL string,
	reads *ethclient.Client,
	readChainID *big.Int,
	metrics *RPCMetrics,
	log logr.Logger,
) (*ethclient.Client, error) {
	endpoint := explicitURL
	if endpoint == "" && len(readURLs) > 1 {
		endpoint = readURLs[0]
	}
	if endpoint == "" {
		return reads, nil
	}
	writes, err := dialRPC(ctx, []string{endpoint}, rpcRoleWrite, metrics, log)
	if err != nil {
		return nil, errors.Errorf("chain: dial write rpc: %w", err)
	}
	if explicitURL == "" {
		return writes, nil
	}
	writeChainID, err := writes.ChainID(ctx)
	if err != nil {
		writes.Close()
		return nil, errors.Errorf("chain: get write rpc chain id: %w", err)
	}
	if writeChainID.Cmp(readChainID) != 0 {
		writes.Close()
		return nil, errors.Errorf(
			"chain: write rpc chain id mismatch: read %s, write %s",
			readChainID,
			writeChainID,
		)
	}
	return writes, nil
}

func (c *Client) ChainID() *big.Int {
	return new(big.Int).Set(c.chainID)
}

func (c *Client) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return c.writes.SendTransaction(ctx, tx)
}

func (c *Client) NonceAt(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (uint64, error) {
	return c.writes.NonceAt(ctx, account, blockNumber)
}

func (c *Client) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return c.writes.PendingNonceAt(ctx, account)
}

// TransactionSenderBalanceAt prefers the write endpoint, then the read plane when a private relay
// does not expose balance reads.
func (c *Client) TransactionSenderBalanceAt(
	ctx context.Context,
	account common.Address,
	blockNumber *big.Int,
) (*big.Int, error) {
	balance, writeErr := c.writes.BalanceAt(ctx, account, blockNumber)
	if writeErr == nil || c.writes == c.Client {
		return balance, writeErr
	}
	balance, readErr := c.BalanceAt(ctx, account, blockNumber)
	if readErr == nil {
		return balance, nil
	}
	return nil, errors.Errorf("chain: transaction sender balance: %w", errors.Join(writeErr, readErr))
}

func (c *Client) Close() {
	c.Client.Close()
	if c.writes != nil && c.writes != c.Client {
		c.writes.Close()
	}
}
