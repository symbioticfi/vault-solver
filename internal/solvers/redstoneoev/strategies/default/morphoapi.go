package defaultstrategy

// morphoapi.go adapts generated Morpho GraphQL responses into OEV-local snapshot types.

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"slices"
	"time"

	"github.com/Khan/genqlient/graphql"
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/api/morphographql"
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

func (a *morphoClient) PositionsByMarket(ctx context.Context, marketIDs []common.Hash, limit int, maxHF *float64) ([]morphoPosition, error) {
	if limit <= 0 || len(marketIDs) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(marketIDs))
	seen := make(map[common.Hash]bool, len(marketIDs))
	for _, id := range marketIDs {
		if !seen[id] {
			ids = append(ids, id.Hex())
			seen[id] = true
		}
	}
	selected := make([]morphoPosition, 0, min(limit, maxPositionsPage))
	for chunk := range slices.Chunk(ids, maxPositionMarketIDs) {
		positions, err := a.positionsChunk(ctx, chunk, limit, maxHF)
		if err != nil {
			return nil, err
		}
		selected = append(selected, positions...)
		sortPositionsByRisk(selected)
		// Once a row is outside the global top limit, later chunks cannot make it relevant.
		if len(selected) > limit {
			clear(selected[limit:])
			selected = selected[:limit]
		}
	}
	return selected, nil
}

// The cap counts upstream rows, including malformed entries. A bad endpoint cannot
// keep a refresh alive by filling every page with unusable positions.
func (a *morphoClient) positionsChunk(ctx context.Context, ids []string, limit int, maxHF *float64) ([]morphoPosition, error) {
	out := make([]morphoPosition, 0, min(limit, maxPositionsPage))
	seen := make(map[positionKey]bool)
	for offset := 0; offset < limit; {
		pageSize := min(maxPositionsPage, limit-offset)
		data, err := morphographql.MorphoPositionsByMarket(ctx, a.gql, ids, pageSize, offset, maxHF)
		if err != nil {
			return nil, err
		}
		items := data.MarketPositions.Items
		if len(items) > pageSize {
			return nil, errors.Errorf("Morpho positions: page has %d rows, requested %d", len(items), pageSize)
		}
		for _, item := range items {
			position := morphoPositionFromAPI(item)
			key := positionKey{market: position.MarketID, borrower: position.Borrower}
			if key.market == (common.Hash{}) || key.borrower == (common.Address{}) || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, position)
		}
		if len(items) < pageSize {
			break
		}
		offset += len(items)
	}
	return out, nil
}

func sortPositionsByRisk(positions []morphoPosition) {
	risk := func(position morphoPosition) float64 {
		if position.HealthFactor == nil {
			return math.Inf(1)
		}
		return *position.HealthFactor
	}
	slices.SortStableFunc(positions, func(left, right morphoPosition) int {
		return cmp.Or(cmp.Compare(risk(left), risk(right)), left.MarketID.Cmp(right.MarketID), left.Borrower.Cmp(right.Borrower))
	})
}

// Generated response types end here. Missing optional states remain missing so
// monitor validation cannot mistake a partially indexed item for a fresh position.
func morphoMarketFromDiscover(item morphographql.MorphoDiscoverMarketsMarketsPaginatedMarketsItemsMarket) morphoMarket {
	market := morphoMarket{MarketID: common.HexToHash(item.MarketId), Oracle: common.HexToAddress(item.OracleAddress),
		IRM: common.HexToAddress(item.IrmAddress), LLTV: item.Lltv.String(), LoanAsset: morphoAsset{Address: common.HexToAddress(item.LoanAsset.Address)}}
	if collateral := item.CollateralAsset; collateral != nil {
		market.CollateralAsset = &morphoAsset{Address: common.HexToAddress(collateral.Address)}
	}
	if state := item.State; state != nil {
		market.State = &morphoMarketState{BlockNumber: state.BlockNumber.String(), BorrowAssets: state.BorrowAssets.String(),
			BorrowShares: state.BorrowShares.String(), SupplyAssets: state.SupplyAssets.String(), SupplyShares: state.SupplyShares.String(), Timestamp: state.Timestamp.String()}
		if state.Price != nil {
			market.State.Price = state.Price.String()
		}
	}
	return market
}

func morphoPositionFromAPI(item morphographql.MorphoPositionsByMarketMarketPositionsPaginatedMarketPositionsItemsMarketPosition) morphoPosition {
	position := morphoPosition{MarketID: common.HexToHash(item.Market.MarketId), Borrower: common.HexToAddress(item.User.Address)}
	if item.HealthFactor != nil {
		risk := *item.HealthFactor
		position.HealthFactor = &risk
	}
	if state := item.State; state != nil {
		position.BorrowShares, position.Collateral = state.BorrowShares.String(), state.Collateral.String()
	}
	return position
}

type boundedGraphQLClient struct {
	url string
	hc  *http.Client
}

// MakeRequest decodes the GraphQL envelope before touching generated data. The
// bounded body and envelope errors apply equally to every generated operation.
func (c boundedGraphQLClient) MakeRequest(ctx context.Context, request *graphql.Request, output *graphql.Response) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return errors.Errorf("morpho graphql: marshal request: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, c.hc.Timeout)
	defer cancel()
	call, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(payload))
	if err != nil {
		return errors.Errorf("morpho graphql: build request: %w", err)
	}
	call.Header.Set("Content-Type", "application/json")
	call.Header.Set("Accept", "application/json")
	response, err := c.hc.Do(call)
	if err != nil {
		return errors.Errorf("morpho graphql: request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.Errorf("morpho graphql: status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxMorphoRespBytes+1))
	if err != nil {
		return errors.Errorf("morpho graphql: read response: %w", err)
	}
	if len(raw) > maxMorphoRespBytes {
		return errors.New("morpho graphql: response too large")
	}
	var envelope graphql.BaseResponse[json.RawMessage]
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return errors.Errorf("morpho graphql: decode response: %w", err)
	}
	output.Errors, output.Extensions = envelope.Errors, envelope.Extensions
	if len(envelope.Errors) != 0 {
		return errors.Errorf("morpho graphql: graphql error: %s", envelope.Errors[0].Message)
	}
	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return errors.New("morpho graphql: response missing data")
	}
	if err := json.Unmarshal(envelope.Data, &output.Data); err != nil {
		return errors.Errorf("morpho graphql: decode response data: %w", err)
	}
	return nil
}
