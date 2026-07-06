package types

import (
	"bytes"
	"encoding/json"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

// RFQ webhook JSON wire contract: big integers are decimal strings, and strategy responses reject
// unknown fields so remote deciders fail closed on schema drift.
type quoteInputJSON struct {
	RequestID         string               `json:"requestId"`
	QuoteID           string               `json:"quoteId"`
	ChainID           int64                `json:"chainId"`
	Executor          common.Address       `json:"executor"`
	TokenIn           common.Address       `json:"tokenIn"`
	TokenOut          common.Address       `json:"tokenOut"`
	AmountIn          string               `json:"amountIn"`
	RequiredAmountOut string               `json:"requiredAmountOut,omitempty"`
	Candidates        []quoteCandidateJSON `json:"candidates"`
	Now               time.Time            `json:"now"`
}

type quoteCandidateJSON struct {
	ID            string         `json:"id"`
	Adapter       common.Address `json:"adapter"`
	Asset         common.Address `json:"asset"`
	AssetDecimals int            `json:"assetDecimals"`
	MaxAssets     string         `json:"maxAssets"`
	MaxRate       string         `json:"maxRate"`
	DiscountID    *common.Hash   `json:"discountId,omitempty"`
}

type quoteOutputJSON struct {
	Decision        Decision       `json:"decision"`
	Reason          string         `json:"reason"`
	QuotedAmountOut string         `json:"quotedAmountOut"`
	Legs            []quoteLegJSON `json:"legs"`
}

type quoteLegJSON struct {
	CandidateID string `json:"candidateId"`
	AmountIn    string `json:"amountIn"`
	AmountOut   string `json:"amountOut"`
}

func (in QuoteInput) MarshalJSON() ([]byte, error) {
	candidates := make([]quoteCandidateJSON, 0, len(in.Candidates))
	for _, c := range in.Candidates {
		candidates = append(candidates, quoteCandidateJSON{
			ID: c.ID, Adapter: c.Adapter, Asset: c.Asset, AssetDecimals: c.AssetDecimals,
			MaxAssets: bigString(c.MaxAssets), MaxRate: bigString(c.MaxRate), DiscountID: c.DiscountID,
		})
	}
	return json.Marshal(quoteInputJSON{
		RequestID: in.RequestID, QuoteID: in.QuoteID, ChainID: in.ChainID,
		Executor: in.Executor, TokenIn: in.TokenIn, TokenOut: in.TokenOut, AmountIn: bigString(in.AmountIn),
		RequiredAmountOut: bigString(in.RequiredAmountOut), Candidates: candidates, Now: in.Now,
	})
}

func (out *QuoteOutput) UnmarshalJSON(b []byte) error {
	var raw quoteOutputJSON
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	quoted, err := parseBigString(raw.QuotedAmountOut, "quotedAmountOut")
	if err != nil {
		return err
	}
	legs := make([]QuoteLeg, 0, len(raw.Legs))
	for i, l := range raw.Legs {
		amountIn, err := parseBigString(l.AmountIn, "legs.amountIn")
		if err != nil {
			return errors.Errorf("leg %d: %w", i, err)
		}
		amountOut, err := parseBigString(l.AmountOut, "legs.amountOut")
		if err != nil {
			return errors.Errorf("leg %d: %w", i, err)
		}
		legs = append(legs, QuoteLeg{CandidateID: l.CandidateID, AmountIn: amountIn, AmountOut: amountOut})
	}
	*out = QuoteOutput{
		Decision:        raw.Decision,
		Reason:          raw.Reason,
		QuotedAmountOut: quoted,
		Legs:            legs,
	}
	return nil
}

func bigString(n *big.Int) string {
	if n == nil {
		return ""
	}
	return n.String()
}

func parseBigString(s, field string) (*big.Int, error) {
	if s == "" {
		return nil, nil
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || n.Sign() < 0 {
		return nil, errors.Errorf("%s: invalid decimal string %q", field, s)
	}
	return n, nil
}
