package merklehost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/recipes"
	"github.com/symbioticfi/vault-solver/api/rfqbackend"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

type indexedRecipeSource struct{ api *rfqbackend.APIClient }

func ValidateIndexerURL(value string) error {
	parsed, parseErr := url.Parse(value)
	if err := permissions.ValidateConfig(protocolio.HTTPConfig{URL: value}); err != nil || parseErr != nil || parsed.Path != "" && parsed.Path != "/" {
		return errors.New("indexerUrl must be an HTTP(S) host URL without credentials, query or fragment")
	}
	return nil
}

// NewRecipeSource uses the indexed-only API published by the RFQ backend.
func NewRecipeSource(url string) (recipes.PublicationSource, error) {
	if err := ValidateIndexerURL(url); err != nil {
		return nil, err
	}
	cfg := rfqbackend.NewConfiguration()
	cfg.Servers = rfqbackend.ServerConfigurations{{URL: strings.TrimRight(url, "/")}}
	cfg.HTTPClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: observability.TraceTransport(recipeResponseLimit{}, "indexed-recipes")}
	return &indexedRecipeSource{api: rfqbackend.NewAPIClient(cfg)}, nil
}

func (s *indexedRecipeSource) RecipePublication(ctx context.Context, executor common.Address, hash common.Hash, published uint64) (recipes.Publication, error) {
	publication, response, err := s.api.RFQAPI.ApiV1RecipesExecutorGet(ctx, strings.ToLower(executor.Hex())).Hash(hash.Hex()).Block(strconv.FormatUint(published, 10)).Execute()
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil || publication == nil {
		return recipes.Publication{}, errors.New("indexed recipe publication unavailable")
	}
	address, err := parse.NonZeroAddress(publication.Executor, "indexed executor")
	if err != nil || address != executor || !strings.EqualFold(publication.RecipeHash, hash.Hex()) {
		return recipes.Publication{}, errors.New("indexed recipe identity mismatch")
	}
	block, err := strconv.ParseUint(publication.BlockNumber, 10, 64)
	if err != nil || block != published || publication.LogIndex < 0 || uint64(publication.LogIndex) > uint64(^uint(0)) {
		return recipes.Publication{}, errors.New("invalid indexed recipe publication position")
	}
	decodeHash := func(value string) (common.Hash, error) {
		raw, err := hexutil.Decode(value)
		if err != nil || len(raw) != 32 || common.BytesToHash(raw) == (common.Hash{}) {
			return common.Hash{}, errors.New("invalid indexed recipe evidence hash")
		}
		return common.BytesToHash(raw), nil
	}
	blockHash, err := decodeHash(publication.BlockHash)
	if err != nil {
		return recipes.Publication{}, err
	}
	txHash, err := decodeHash(publication.TransactionHash)
	if err != nil {
		return recipes.Publication{}, err
	}
	decodeProgram := func(encoded []string) ([][]byte, error) {
		if len(encoded) > 4096 {
			return nil, errors.New("indexed recipe exceeds program bound")
		}
		out := make([][]byte, len(encoded))
		for i, value := range encoded {
			raw, err := hexutil.Decode(value)
			if err != nil {
				return nil, errors.New("invalid indexed recipe bytes")
			}
			out[i] = raw
		}
		return out, nil
	}
	queries, err := decodeProgram(publication.Queries)
	if err != nil {
		return recipes.Publication{}, err
	}
	steps, err := decodeProgram(publication.Steps)
	if err != nil {
		return recipes.Publication{}, err
	}
	actual, err := recipes.HashRecipe(recipes.Recipe{Queries: queries, Steps: steps})
	if err != nil || actual != hash {
		return recipes.Publication{}, errors.New("indexed recipe body differs from current commitment")
	}
	return recipes.Publication{Executor: executor, RecipeHash: hash, BlockNumber: block, BlockHash: blockHash, TransactionHash: txHash, LogIndex: uint(publication.LogIndex), Recipe: recipes.Recipe{Queries: queries, Steps: steps}}, nil
}

type recipeResponseLimit struct{}

func (recipeResponseLimit) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, permissions.HostMaxResponseBytes+1))
	_ = response.Body.Close()
	if err != nil || int64(len(body)) > permissions.HostMaxResponseBytes {
		return nil, errors.New("indexed recipe response unavailable or oversized")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
