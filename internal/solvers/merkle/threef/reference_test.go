package threef

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
)

//go:embed reference/guard.json
var referenceJSON []byte

func referenceGuard(account common.Address, market mintMarket) (data, mask []byte, err error) {
	var ref struct {
		Data hexutil.Bytes `json:"mintLeafData"`
		Mask hexutil.Bytes `json:"mintLeafMask"`
	}
	if err := json.Unmarshal(referenceJSON, &ref); err != nil {
		return nil, nil, err
	}
	decoded, err := capabilitySchema.Methods["execute"].Inputs.Unpack(ref.Data[4:])
	if err != nil {
		return nil, nil, err
	}
	queries, steps := decoded[0].([][]byte), decoded[1].([][]byte)
	for i, q := range queries {
		args, e := queryArgs.Unpack(q)
		if e != nil {
			return nil, nil, e
		}
		call := bytes.Clone(args[1].([]byte))
		if i != 3 && i != 5 {
			copy(call[4:36], common.LeftPadBytes(account.Bytes(), 32))
		}
		switch i {
		case 2, 4:
			args[0] = market.Asset
		case 5:
			args[0] = market.Policy
			call = policyABI.PackRemainingAssets(market.Asset, market.UnderlyingAsset)
		}
		args[1] = call
		queries[i], err = queryArgs.Pack(args...)
		if err != nil {
			return nil, nil, err
		}
	}
	copy(steps[1][33:65], common.LeftPadBytes(market.Asset.Bytes(), 32))
	for _, i := range []int{10, 11, 12, 16} {
		args, e := callArgs.Unpack(steps[i][1:])
		if e != nil {
			return nil, nil, e
		}
		args[0] = market.Asset
		if i == 10 {
			args[0] = market.Policy
		}
		packed, e := callArgs.Pack(args...)
		if e != nil {
			return nil, nil, e
		}
		steps[i] = append([]byte{recipes.Call}, packed...)
	}
	packed, err := capability.TryPackExecute(queries, steps, decoded[2].([]*big.Int))
	return packed, ref.Mask, err
}
