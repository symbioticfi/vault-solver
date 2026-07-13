package defaultstrategy

// morphoapi.go adapts generated Morpho GraphQL responses into OEV-local snapshot types.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/morphographql"
	"github.com/symbioticfi/vault-solver/api/morphographql/scalars"
)

// maxDiscoverMarkets bounds the candidate markets one discovery poll proposes (the `first` arg + a defensive
// truncation): a token pair has few real markets, so this only stops a misbehaving/compromised endpoint
// from flooding the local snapshot — far above any honest (loan, collateral) market count.
const maxDiscoverMarkets = 512

// maxMorphoRespBytes bounds the response body we read from the external endpoint (defence in depth atop the
// client Timeout): a misbehaving/compromised endpoint can't drive unbounded allocation in the discovery
// goroutine. The at-risk band for our market set is small; 8 MiB is far above any honest response.
const maxMorphoRespBytes = 8 << 20

// Live Morpho API request caps observed on 2026-06-26: `marketPositions(first:1001)` and
// `marketUniqueKey_in` with 101 ids both fail input validation. Keep each HTTP response small and page in
// the wrapper so solver config can express a larger logical cap.
const (
	maxPositionsPage     = 1000
	maxPositionMarketIDs = 100
)

// morphoClient is the OEV-local adapter over the generated Morpho GraphQL binding.
type morphoClient struct {
	gql graphql.Client
}

type morphoMarket struct {
	MarketID        common.Hash
	Oracle          common.Address
	IRM             common.Address
	LLTV            string
	LoanAsset       morphoAsset
	CollateralAsset *morphoAsset
	State           *morphoMarketState
}

type morphoAsset struct {
	Address common.Address
}

type morphoMarketState struct {
	BlockNumber  string
	BorrowAssets string
	BorrowShares string
	SupplyAssets string
	SupplyShares string
	Timestamp    string
	Price        string
}

type morphoPosition struct {
	MarketID     common.Hash
	Borrower     common.Address
	HealthFactor *float64
	BorrowShares string
	Collateral   string
}

func newMorphoClient(url string) *morphoClient {
	hc := &http.Client{Timeout: 8 * time.Second}
	return &morphoClient{
		gql: boundedGraphQLClient{url: url, hc: hc},
	}
}

// DiscoverMarketData returns Morpho markets plus their latest indexed state for adapter-derived token
// pairs. Callers must still fail closed on malformed items and verify that the derived market id matches the
// returned id before using the data for execution.
func (a *morphoClient) DiscoverMarketData(ctx context.Context, chainID int64, loan, collateral []common.Address) ([]morphoMarket, error) {
	if len(loan) == 0 || len(collateral) == 0 {
		return nil, nil
	}
	data, err := morphographql.MorphoDiscoverMarkets(ctx, a.gql, lowerAddresses(loan), lowerAddresses(collateral), []int{int(chainID)}, maxDiscoverMarkets)
	if err != nil {
		return nil, err
	}
	out := make([]morphoMarket, 0, min(len(data.Markets.Items), maxDiscoverMarkets))
	for i := range data.Markets.Items {
		if len(out) >= maxDiscoverMarkets {
			break
		}
		m := morphoMarketFromDiscover(data.Markets.Items[i])
		if m.MarketID == (common.Hash{}) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (a *morphoClient) PositionsByMarket(ctx context.Context, marketIDs []common.Hash, first int, maxHF *float64) ([]morphoPosition, error) {
	if len(marketIDs) == 0 || first <= 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(marketIDs))
	for _, id := range marketIDs {
		ids = append(ids, lowerHash(id))
	}
	out := make([]morphoPosition, 0, min(first, maxPositionsPage))
	for start := 0; start < len(ids); start += maxPositionMarketIDs {
		end := min(start+maxPositionMarketIDs, len(ids))
		page, err := a.positionsChunk(ctx, ids[start:end], first, maxHF)
		if err != nil {
			return nil, err
		}
		out = append(out, page...)
	}
	sortPositionsByRisk(out)
	if len(out) > first {
		out = out[:first]
	}
	return out, nil
}

func (a *morphoClient) positionsChunk(ctx context.Context, ids []string, limit int, maxHF *float64) ([]morphoPosition, error) {
	out := make([]morphoPosition, 0, min(limit, maxPositionsPage))
	for skip := 0; len(out) < limit; {
		first := min(maxPositionsPage, limit-len(out))
		data, err := morphographql.MorphoPositionsByMarket(ctx, a.gql, ids, first, skip, maxHF)
		if err != nil {
			return nil, err
		}
		items := data.MarketPositions.Items
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			var state wirePositionState
			if it.State != nil {
				state = it.State
			}
			pos := morphoPositionFromWire(it.User.Address, it.Market.MarketId, it.HealthFactor, state)
			if pos.MarketID == (common.Hash{}) || pos.Borrower == (common.Address{}) {
				continue
			}
			out = append(out, pos)
		}
		if len(items) < first {
			break
		}
		skip += len(items)
	}
	return out, nil
}

func sortPositionsByRisk(pos []morphoPosition) {
	slices.SortStableFunc(pos, func(a, b morphoPosition) int {
		if a.HealthFactor != nil && b.HealthFactor != nil && *a.HealthFactor != *b.HealthFactor {
			if *a.HealthFactor < *b.HealthFactor {
				return -1
			}
			return 1
		}
		if a.HealthFactor == nil && b.HealthFactor != nil {
			return 1
		}
		if a.HealthFactor != nil && b.HealthFactor == nil {
			return -1
		}
		if c := a.MarketID.Cmp(b.MarketID); c != 0 {
			return c
		}
		return a.Borrower.Cmp(b.Borrower)
	})
}

type wireAsset interface {
	GetAddress() string
}

type wireMarketState interface {
	GetBlockNumber() scalars.BigIntString
	GetBorrowAssets() scalars.BigIntString
	GetBorrowShares() scalars.BigIntString
	GetSupplyAssets() scalars.BigIntString
	GetSupplyShares() scalars.BigIntString
	GetTimestamp() scalars.BigIntString
	GetPrice() *scalars.BigIntString
}

func morphoMarketFromDiscover(it morphographql.MorphoDiscoverMarketsMarketsPaginatedMarketsItemsMarket) morphoMarket {
	var coll wireAsset
	if it.CollateralAsset != nil {
		coll = it.CollateralAsset
	}
	var state wireMarketState
	if it.State != nil {
		state = it.State
	}
	return morphoMarketFromWire(it.MarketId, it.OracleAddress, it.IrmAddress, it.Lltv.String(), &it.LoanAsset, coll, state)
}

func morphoMarketFromWire(id, oracle, irm, lltv string, loan wireAsset, collateral wireAsset, state wireMarketState) morphoMarket {
	m := morphoMarket{
		MarketID:  common.HexToHash(id),
		Oracle:    common.HexToAddress(oracle),
		IRM:       common.HexToAddress(irm),
		LLTV:      lltv,
		LoanAsset: morphoAssetFromWire(loan),
	}
	if collateral != nil {
		c := morphoAssetFromWire(collateral)
		m.CollateralAsset = &c
	}
	if state != nil {
		m.State = morphoMarketStateFromWire(state)
	}
	return m
}

func morphoAssetFromWire(a wireAsset) morphoAsset {
	if a == nil {
		return morphoAsset{}
	}
	return morphoAsset{Address: common.HexToAddress(a.GetAddress())}
}

func morphoMarketStateFromWire(s wireMarketState) *morphoMarketState {
	st := &morphoMarketState{
		BlockNumber:  s.GetBlockNumber().String(),
		BorrowAssets: s.GetBorrowAssets().String(),
		BorrowShares: s.GetBorrowShares().String(),
		SupplyAssets: s.GetSupplyAssets().String(),
		SupplyShares: s.GetSupplyShares().String(),
		Timestamp:    s.GetTimestamp().String(),
	}
	if p := s.GetPrice(); p != nil {
		st.Price = p.String()
	}
	return st
}

type wirePositionState interface {
	GetBorrowShares() scalars.BigIntString
	GetCollateral() scalars.BigIntString
}

func morphoPositionFromWire(user, market string, health *float64, state wirePositionState) morphoPosition {
	pos := morphoPosition{
		MarketID:     common.HexToHash(market),
		Borrower:     common.HexToAddress(user),
		HealthFactor: health,
	}
	if state == nil {
		return pos
	}
	pos.BorrowShares = state.GetBorrowShares().String()
	pos.Collateral = state.GetCollateral().String()
	return pos
}

type boundedGraphQLClient struct {
	url string
	hc  *http.Client
}

// MakeRequest is genqlient's transport hook. It keeps the old fail-safe HTTP behavior while the query and
// response types come from generated code.
func (c boundedGraphQLClient) MakeRequest(ctx context.Context, req *graphql.Request, resp *graphql.Response) error {
	body, err := json.Marshal(req)
	if err != nil {
		return errors.Errorf("morpho graphql: marshal request: %w", err)
	}
	reqCtx, cancel := context.WithTimeout(ctx, c.hc.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return errors.Errorf("morpho graphql: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	httpResp, err := c.hc.Do(httpReq)
	if err != nil {
		return errors.Errorf("morpho graphql: request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()
	if httpResp.StatusCode != http.StatusOK {
		return errors.Errorf("morpho graphql: status %d", httpResp.StatusCode)
	}

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxMorphoRespBytes+1))
	if err != nil {
		return errors.Errorf("morpho graphql: read response: %w", err)
	}
	if len(raw) > maxMorphoRespBytes {
		return errors.New("morpho graphql: response too large")
	}
	if err := json.Unmarshal(raw, resp); err != nil {
		return errors.Errorf("morpho graphql: decode response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return errors.Errorf("morpho graphql: graphql error: %s", resp.Errors[0].Message)
	}

	var envelope struct {
		Data *json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return errors.Errorf("morpho graphql: decode response envelope: %w", err)
	}
	if envelope.Data == nil || bytes.Equal(bytes.TrimSpace(*envelope.Data), []byte("null")) {
		return errors.New("morpho graphql: response missing data")
	}
	return nil
}
