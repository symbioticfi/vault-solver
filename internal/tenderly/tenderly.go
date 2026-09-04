// Package tenderly creates inert simulator draft links for failed transaction diagnostics.
package tenderly

import (
	"encoding/base64"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const simulatorBaseURL = "https://dashboard.tenderly.co/simulator/new"

type draft struct {
	V       int     `json:"v"`
	Network network `json:"network"`
	Row     row     `json:"row"`
}

type network struct {
	ID string `json:"id"`
}

type row struct {
	ContractAddress  string `json:"contractAddress"`
	From             string `json:"from"`
	InputDataType    string `json:"inputDataType"`
	RawFunctionInput string `json:"rawFunctionInput"`
	Value            string `json:"value"`
}

func SimulatorURL(
	chainID *big.Int,
	from common.Address,
	to common.Address,
	calldata []byte,
	value *big.Int,
) string {
	if chainID == nil {
		return ""
	}
	amount := "0"
	if value != nil {
		amount = value.String()
	}
	payload, err := json.Marshal(draft{
		V:       1,
		Network: network{ID: chainID.String()},
		Row: row{
			ContractAddress:  to.Hex(),
			From:             from.Hex(),
			InputDataType:    "raw",
			RawFunctionInput: hexutil.Encode(calldata),
			Value:            amount,
		},
	})
	if err != nil {
		return ""
	}
	return simulatorBaseURL + "?draft=" + base64.RawURLEncoding.EncodeToString(payload)
}
