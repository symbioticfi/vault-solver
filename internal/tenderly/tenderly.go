// Package tenderly builds Tenderly Simulator draft links so a failed transaction can be replayed
// with its exact calldata (no ABI needed) for debugging.
package tenderly

import (
	"encoding/base64"
	"encoding/json"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

const simulatorBaseURL = "https://dashboard.tenderly.co/simulator/new"

// draft is the payload Tenderly's Simulator expects base64url-encoded in the `?draft=` query param:
// https://docs.tenderly.co/simulator-ui/draft-links
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

// SimulatorURL returns a Tenderly Simulator draft link for a single raw call (to/from/calldata on
// chainID), or "" if chainID is nil. The draft only pre-fills the simulator; it executes nothing
// until opened and run.
func SimulatorURL(chainID *big.Int, from, to common.Address, data []byte, value *big.Int) string {
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
			RawFunctionInput: hexutil.Encode(data),
			Value:            amount,
		},
	})
	if err != nil {
		return ""
	}
	return simulatorBaseURL + "?draft=" + base64.RawURLEncoding.EncodeToString(payload)
}
