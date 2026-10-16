package chain

import (
	"context"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/vault-solver/api/bindings/multicall3"
)

// BatchRead is the SDK's pinned read boundary. Ordinary solver RPC methods and
// their latest-state Multicall helper retain their existing behavior.
func (c *Client) BatchRead(ctx context.Context, requests []protocolio.ReadRequest) ([]protocolio.ReadResult, error) {
	reader := protocolio.RPCBatchReader{Call: c.sdkBatchCall, Multicall: common.HexToAddress(protocolio.StandardMulticall3), Codec: sdkAggregateCodec{}}
	return reader.BatchRead(ctx, requests)
}

func (c *Client) sdkBatchCall(ctx context.Context, batch []rpc.BatchElem) error {
	if c.readHTTP == nil {
		return errors.New("chain: SDK read batching requires HTTP(S) read endpoints")
	}
	return protocolio.CallHTTPBatch(ctx, c.readHTTP, c.readURL, batch)
}

type sdkAggregateCodec struct{}

func (sdkAggregateCodec) Encode(calls []protocolio.AggregateCall) ([]byte, error) {
	subcalls := make([]multicall3.Multicall3Call3, len(calls))
	for i, call := range calls {
		subcalls[i] = multicall3.Multicall3Call3{Target: call.Target, AllowFailure: true, CallData: call.Data}
	}
	return multicallB.TryPackAggregate3(subcalls)
}
func (sdkAggregateCodec) Decode(raw []byte) ([]protocolio.AggregateResult, error) {
	decoded, err := multicallB.UnpackAggregate3(raw)
	if err != nil {
		return nil, err
	}
	results := make([]protocolio.AggregateResult, len(decoded))
	for i, result := range decoded {
		results[i] = protocolio.AggregateResult{Success: result.Success, Data: result.ReturnData}
	}
	return results, nil
}

// CallContractWithCode preserves the recipe lens's sender, target, block and
// state overrides through the same read transport as ordinary eth_call.
func (c *Client) CallContractWithCode(ctx context.Context, msg ethereum.CallMsg, block *big.Int, code []byte) (_ []byte, err error) {
	ctx, end := c.readCalls.start(ctx, rpcMethodCall)
	defer func() { end(err) }()
	if msg.To == nil || *msg.To == (common.Address{}) || msg.From == (common.Address{}) || block == nil || block.Sign() < 0 || len(code) == 0 {
		return nil, errors.New("chain: code override requires target, caller, numeric block and code")
	}
	overrides := map[common.Address]any{*msg.To: map[string]any{"code": hexutil.Bytes(code)}}
	var result hexutil.Bytes
	err = c.Client.Client().CallContext(ctx, &result, "eth_call", protocolio.CallArgument(msg), protocolio.BlockArgument(block), overrides)
	return result, err
}
