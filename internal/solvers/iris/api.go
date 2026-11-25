package iris

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

// Wire types follow the IRIS RFQ API reference (Solver webhook). IRIS publishes no OpenAPI
// document, so they are hand-written; numbers are decimal strings except chainId.

const (
	// maxVenueID is the largest venue index Iris accepts in a Quote.
	maxVenueID = 127
	// placeholderSignatureBytes is the shortest signature the coordinator accepts. The account's
	// isValidSignature ignores it and checks only the stored digest.
	placeholderSignatureBytes = 65
	maxRequestBytes           = 16 << 10
)

type quoteRequest struct {
	RequestID       string `json:"requestId"`
	QuoteID         string `json:"quoteId"`
	ChainID         int64  `json:"chainId"`
	Borrower        string `json:"borrower"`
	Receiver        string `json:"receiver"`
	CollateralToken string `json:"collateralToken"`
	Collateral      string `json:"collateral"`
	DebtToken       string `json:"debtToken"`
	Debt            string `json:"debt"`
	MaxFixedRate    string `json:"maxFixedRate"`
	Duration        string `json:"duration"`
	VenueBitmap     string `json:"venueBitmap"`
}

type quoteResponse struct {
	RequestID     string `json:"requestId"`
	QuoteID       string `json:"quoteId,omitempty"`
	Solver        string `json:"solver"`
	Blm           string `json:"blm"`
	FixedRate     string `json:"fixedRate"`
	VenueID       string `json:"venueId"`
	Data          string `json:"data"`
	OverdueRate   string `json:"overdueRate"`
	OverduePeriod string `json:"overduePeriod"`
	BondLltv      string `json:"bondLltv"`
	Bond          string `json:"bond"`
	Deadline      string `json:"deadline,omitempty"`
	Nonce         string `json:"nonce,omitempty"`
	Signature     string `json:"signature,omitempty"`
	Indicative    bool   `json:"indicative,omitempty"`
}

type declineReason struct {
	Venue  string `json:"venue"`
	Reason string `json:"reason"`
	Rate   string `json:"rate,omitempty"`
}

type declineBody struct {
	Reasons []declineReason `json:"reasons,omitempty"`
}

// request is a validated quote request.
type request struct {
	RequestID, QuoteID             string
	Borrower, Receiver             common.Address
	CollateralToken, DebtToken     common.Address
	Collateral, Debt, MaxFixedRate *big.Int
	Duration, VenueBitmap          *big.Int
}

// parseRequest validates the coordinator's body. Indicative requests may omit the
// borrower and receiver.
func parseRequest(body quoteRequest, chainID *big.Int, indicative bool) (request, error) {
	if body.RequestID == "" {
		return request{}, errors.New("requestId required")
	}
	if !chainID.IsInt64() || body.ChainID != chainID.Int64() {
		return request{}, errors.Errorf("chainId %d is not this chain", body.ChainID)
	}
	r := request{RequestID: body.RequestID, QuoteID: body.QuoteID}
	var err error
	for _, field := range []struct {
		value, name string
		target      *common.Address
	}{{body.Borrower, "borrower", &r.Borrower}, {body.Receiver, "receiver", &r.Receiver}} {
		if field.value == "" && indicative {
			continue
		}
		if *field.target, err = parse.NonZeroAddress(field.value, field.name); err != nil {
			return request{}, err
		}
	}
	if r.CollateralToken, err = parse.NonZeroAddress(body.CollateralToken, "collateralToken"); err != nil {
		return request{}, err
	}
	if r.DebtToken, err = parse.NonZeroAddress(body.DebtToken, "debtToken"); err != nil {
		return request{}, err
	}
	for _, field := range []struct {
		value  string
		name   string
		target **big.Int
	}{
		{body.Collateral, "collateral", &r.Collateral}, {body.Debt, "debt", &r.Debt},
		{body.MaxFixedRate, "maxFixedRate", &r.MaxFixedRate}, {body.Duration, "duration", &r.Duration},
		{body.VenueBitmap, "venueBitmap", &r.VenueBitmap},
	} {
		value, err := parse.Big(field.value, field.name)
		if err != nil {
			return request{}, err
		}
		if value.Sign() <= 0 || value.BitLen() > 256 {
			return request{}, errors.Errorf("%s must be a positive uint256", field.name)
		}
		*field.target = value
	}
	return r, nil
}

func newQuoteResponse(r request, m Market, q Quote, indicative bool) quoteResponse {
	response := quoteResponse{
		RequestID: r.RequestID, QuoteID: r.QuoteID, Solver: m.Account.Hex(), Blm: m.Blm.Hex(),
		FixedRate: q.FixedRate.String(), VenueID: q.VenueID.String(), Data: hexutil.Encode(q.Data),
		OverdueRate: q.OverdueRate.String(), OverduePeriod: q.OverduePeriod.String(),
		BondLltv: q.BondLltv.String(), Bond: q.Bond.String(), Indicative: indicative,
	}
	if !indicative {
		response.Deadline, response.Nonce = q.Deadline.String(), q.Nonce.String()
		response.Signature = "0x" + strings.Repeat("00", placeholderSignatureBytes)
	}
	return response
}
