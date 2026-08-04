package rfq

import (
	"math/big"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/google/uuid"

	"github.com/symbioticfi/vault-solver/internal/liquidlane"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

type swapPhase string

const (
	swapProtocolV2                = "v2"
	swapPhaseDiscovery  swapPhase = "DISCOVERY"
	swapPhaseConfirm    swapPhase = "CONFIRM"
	swapPhaseBuild      swapPhase = "BUILD"
	maxDiscoverySamples           = 32
	maxUint48                     = int64(1<<48 - 1)
)

type swapRequest struct {
	Protocol           string         `json:"protocol" enum:"v2"`
	Phase              swapPhase      `json:"phase" enum:"DISCOVERY,CONFIRM,BUILD"`
	RequestID          string         `json:"requestId" format:"uuid"`
	DiscoveryRequestID *string        `json:"discoveryRequestId,omitempty" format:"uuid"`
	QuoteID            string         `json:"quoteId" format:"uuid"`
	SolverQuoteID      *string        `json:"solverQuoteId,omitempty" format:"uuid"`
	BuildID            *string        `json:"buildId,omitempty" format:"uuid"`
	ChainID            int64          `json:"chainId" minimum:"1"`
	Swapper            string         `json:"swapper" pattern:"^0x[a-fA-F0-9]{40}$"`
	TokenIn            string         `json:"tokenIn" pattern:"^0x[a-fA-F0-9]{40}$"`
	TokenOut           string         `json:"tokenOut" pattern:"^0x[a-fA-F0-9]{40}$"`
	SampleAmountsIn    []string       `json:"sampleAmountsIn,omitempty" maxItems:"32"`
	AmountIn           *string        `json:"amountIn,omitempty" pattern:"^[0-9]+$"`
	MinAmountOut       *string        `json:"minAmountOut,omitempty" pattern:"^[0-9]+$"`
	Deadline           *int64         `json:"deadline,omitempty"`
	Adapters           []quoteAdapter `json:"adapters,omitempty" maxItems:"256"`
	LiquidityDomains   []string       `json:"liquidityDomains,omitempty" maxItems:"256"`
	Router             *string        `json:"router,omitempty" pattern:"^0x[a-fA-F0-9]{40}$"`
}

type swapPointResponse struct {
	AmountIn         string   `json:"amountIn"`
	AmountOut        string   `json:"amountOut"`
	LiquidityDomains []string `json:"liquidityDomains"`
}

type swapCallResponse struct {
	To              string `json:"to"`
	Data            string `json:"data"`
	AuthSigner      string `json:"authSigner"`
	AuthDeadline    int64  `json:"authDeadline"`
	AuthSignature   string `json:"authSignature"`
	AmountIn        string `json:"amountIn"`
	AmountOut       string `json:"amountOut"`
	TokenOut        string `json:"tokenOut"`
	LiquidityDomain string `json:"liquidityDomain"`
	ValidUntil      int64  `json:"validUntil"`
}

type swapResponse struct {
	Protocol           string               `json:"protocol"`
	Phase              swapPhase            `json:"phase"`
	RequestID          string               `json:"requestId"`
	DiscoveryRequestID string               `json:"discoveryRequestId,omitempty"`
	QuoteID            string               `json:"quoteId"`
	SolverQuoteID      string               `json:"solverQuoteId,omitempty"`
	BuildID            string               `json:"buildId,omitempty"`
	ChainID            int64                `json:"chainId"`
	Swapper            string               `json:"swapper"`
	Router             string               `json:"router,omitempty"`
	TokenIn            string               `json:"tokenIn"`
	TokenOut           string               `json:"tokenOut"`
	Points             *[]swapPointResponse `json:"points,omitempty"`
	AmountIn           string               `json:"amountIn,omitempty"`
	AmountOut          string               `json:"amountOut,omitempty"`
	LiquidityDomains   []string             `json:"liquidityDomains,omitempty"`
	ValidUntil         int64                `json:"validUntil,omitempty"`
	Calls              *[]swapCallResponse  `json:"calls,omitempty"`
}

type parsedSwapRequest struct {
	Protocol           string
	Phase              swapPhase
	RequestID          uuid.UUID
	DiscoveryRequestID uuid.UUID
	QuoteID            uuid.UUID
	SolverQuoteID      uuid.UUID
	BuildID            uuid.UUID
	ChainID            int64
	Swapper            common.Address
	TokenIn            common.Address
	TokenOut           common.Address
	SampleAmountsIn    []*big.Int
	AmountIn           *big.Int
	MinAmountOut       *big.Int
	Deadline           time.Time
	Inventory          []solverInventory
	LiquidityDomains   []liquidlane.CapacityID
	Router             common.Address
}

type exactSwapFields struct {
	AmountIn     *big.Int
	MinAmountOut *big.Int
	Deadline     time.Time
}

func (r *swapRequest) parse(chainID int64, configuredRouter common.Address) (*parsedSwapRequest, error) {
	if r.Protocol != swapProtocolV2 {
		return nil, errors.Errorf("protocol must be %q", swapProtocolV2)
	}
	if r.Phase != swapPhaseDiscovery && r.Phase != swapPhaseConfirm && r.Phase != swapPhaseBuild {
		return nil, errors.Errorf("invalid phase %q", r.Phase)
	}
	requestID, err := parseCanonicalUUID(r.RequestID, "requestId")
	if err != nil {
		return nil, err
	}
	quoteID, err := parseCanonicalUUID(r.QuoteID, "quoteId")
	if err != nil {
		return nil, err
	}
	if r.ChainID != chainID {
		return nil, errors.Errorf("chainId must be %d", chainID)
	}
	swapper, err := parse.NonZeroAddress(r.Swapper, "swapper")
	if err != nil {
		return nil, err
	}
	tokenIn, err := parse.NonZeroAddress(r.TokenIn, "tokenIn")
	if err != nil {
		return nil, err
	}
	tokenOut, err := parse.NonZeroAddress(r.TokenOut, "tokenOut")
	if err != nil {
		return nil, err
	}
	if tokenIn == tokenOut {
		return nil, errors.New("tokenIn and tokenOut must differ")
	}

	parsed := &parsedSwapRequest{
		Protocol: r.Protocol, Phase: r.Phase, RequestID: requestID, QuoteID: quoteID, ChainID: r.ChainID,
		Swapper: swapper, TokenIn: tokenIn, TokenOut: tokenOut,
	}
	switch r.Phase {
	case swapPhaseDiscovery:
		if r.DiscoveryRequestID != nil || r.SolverQuoteID != nil || r.BuildID != nil || r.AmountIn != nil ||
			r.MinAmountOut != nil || r.Deadline != nil || len(r.LiquidityDomains) != 0 || r.Router != nil {
			return nil, errors.New("DISCOVERY contains fields from another phase")
		}
		parsed.SampleAmountsIn, err = parseSampleAmounts(r.SampleAmountsIn)
		if err != nil {
			return nil, err
		}
		parsed.Inventory, err = parseSwapAdapters(r.Adapters, r.ChainID, tokenIn, tokenOut)
	case swapPhaseConfirm:
		if r.DiscoveryRequestID == nil || r.SolverQuoteID != nil || r.BuildID != nil ||
			len(r.SampleAmountsIn) != 0 || len(r.LiquidityDomains) != 0 || r.Router != nil {
			return nil, errors.New("CONFIRM has missing or inadmissible phase fields")
		}
		parsed.DiscoveryRequestID, err = parseCanonicalUUID(*r.DiscoveryRequestID, "discoveryRequestId")
		if err == nil {
			var exact exactSwapFields
			exact, err = parseExactSwapFields(r)
			parsed.AmountIn, parsed.MinAmountOut, parsed.Deadline = exact.AmountIn, exact.MinAmountOut, exact.Deadline
		}
		if err == nil {
			parsed.Inventory, err = parseSwapAdapters(r.Adapters, r.ChainID, tokenIn, tokenOut)
		}
	case swapPhaseBuild:
		if r.DiscoveryRequestID != nil || r.SolverQuoteID == nil || r.BuildID == nil ||
			len(r.SampleAmountsIn) != 0 || len(r.Adapters) != 0 || r.Router == nil {
			return nil, errors.New("BUILD has missing or inadmissible phase fields")
		}
		parsed.SolverQuoteID, err = parseCanonicalUUID(*r.SolverQuoteID, "solverQuoteId")
		if err == nil {
			parsed.BuildID, err = parseCanonicalUUID(*r.BuildID, "buildId")
		}
		if err == nil {
			var exact exactSwapFields
			exact, err = parseExactSwapFields(r)
			parsed.AmountIn, parsed.MinAmountOut, parsed.Deadline = exact.AmountIn, exact.MinAmountOut, exact.Deadline
		}
		if err == nil {
			parsed.LiquidityDomains, err = parseCapacityDomains(r.LiquidityDomains, r.ChainID)
		}
		if err == nil {
			parsed.Router, err = parse.NonZeroAddress(*r.Router, "router")
		}
		if err == nil && parsed.Router != configuredRouter {
			err = errors.Errorf("router must be %s", lowerAddr(configuredRouter))
		}
	}
	if err != nil {
		return nil, err
	}
	return parsed, nil
}

func parseCanonicalUUID(raw, field string) (uuid.UUID, error) {
	value, err := uuid.Parse(raw)
	if err != nil || value == uuid.Nil || value.String() != raw {
		return uuid.Nil, errors.Errorf("%s: invalid canonical UUID %q", field, raw)
	}
	return value, nil
}

func parseSampleAmounts(raw []string) ([]*big.Int, error) {
	if len(raw) == 0 || len(raw) > maxDiscoverySamples {
		return nil, errors.Errorf("sampleAmountsIn must contain 1 to %d values", maxDiscoverySamples)
	}
	out := make([]*big.Int, len(raw))
	for i, value := range raw {
		amount, err := parseUint256(value, "sampleAmountsIn["+strconv.Itoa(i)+"]")
		if err != nil || amount.Sign() <= 0 {
			return nil, errors.Errorf("sampleAmountsIn[%d] must be positive", i)
		}
		if i > 0 && amount.Cmp(out[i-1]) <= 0 {
			return nil, errors.New("sampleAmountsIn must be strictly increasing and duplicate-free")
		}
		out[i] = amount
	}
	return out, nil
}

func parseCapacityDomains(raw []string, chainID int64) ([]liquidlane.CapacityID, error) {
	if len(raw) == 0 {
		return nil, errors.New("liquidityDomains must not be empty")
	}
	seen := make(map[string]bool, len(raw))
	out := make([]liquidlane.CapacityID, 0, len(raw))
	for i, domain := range raw {
		parts := strings.Split(domain, ":")
		if domain != strings.ToLower(domain) || len(parts) != 4 || parts[0] != "capacity" {
			return nil, errors.Errorf("liquidityDomains[%d]: invalid capacity domain", i)
		}
		domainChain, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || domainChain != chainID {
			return nil, errors.Errorf("liquidityDomains[%d]: wrong chain", i)
		}
		vault, err := parse.NonZeroAddress(parts[2], "liquidityDomains["+strconv.Itoa(i)+"].vault")
		if err != nil {
			return nil, err
		}
		tokenOut, err := parse.NonZeroAddress(parts[3], "liquidityDomains["+strconv.Itoa(i)+"].tokenOut")
		if err != nil {
			return nil, err
		}
		canonical := string(liquidlane.NewCapacityID(chainID, vault, tokenOut))
		if canonical != domain || seen[canonical] {
			return nil, errors.Errorf("liquidityDomains[%d]: non-canonical or duplicate domain", i)
		}
		seen[canonical] = true
		out = append(out, liquidlane.CapacityID(canonical))
	}
	slices.Sort(out)
	return out, nil
}

func parseSwapDeadline(raw *int64, field string) (time.Time, error) {
	if raw == nil || *raw <= 0 || *raw > maxUint48 {
		return time.Time{}, errors.Errorf("%s must be a positive uint48 Unix timestamp", field)
	}
	return time.Unix(*raw, 0), nil
}

func parseExactSwapFields(r *swapRequest) (exactSwapFields, error) {
	if r.AmountIn == nil || r.MinAmountOut == nil {
		return exactSwapFields{}, errors.New("amountIn and minAmountOut are required")
	}
	amountIn, err := parseUint256(*r.AmountIn, "amountIn")
	if err != nil || amountIn.Sign() <= 0 {
		return exactSwapFields{}, errors.New("amountIn must be positive")
	}
	minAmountOut, err := parseUint256(*r.MinAmountOut, "minAmountOut")
	if err != nil || minAmountOut.Sign() <= 0 {
		return exactSwapFields{}, errors.New("minAmountOut must be positive")
	}
	deadline, err := parseSwapDeadline(r.Deadline, "deadline")
	if err != nil {
		return exactSwapFields{}, err
	}
	return exactSwapFields{AmountIn: amountIn, MinAmountOut: minAmountOut, Deadline: deadline}, nil
}

func parseSwapAdapters(raw []quoteAdapter, chainID int64, tokenIn, tokenOut common.Address) ([]solverInventory, error) {
	out := make([]solverInventory, 0, len(raw))
	for i := range raw {
		entry, err := raw[i].parse(i, chainID, tokenIn)
		if err != nil {
			return nil, err
		}
		if entry.Adapter == (common.Address{}) || entry.TokenOut != tokenOut {
			return nil, errors.Errorf("adapters[%d] does not match the requested token pair", i)
		}
		out = append(out, entry)
	}
	return out, nil
}
