package bridgefacilitator

import (
	"context"
	"math"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// getOffersDeadlineWindow is how far in the future the signed GetOffers deadline is set.
const getOffersDeadlineWindow = 5 * time.Minute

const maxExactFloat32Int = 1 << 24

// apiClient wraps the generated 3F client. It signs per-adapter requests via EIP-712 and injects
// the resulting Authorization: Bearer header.
//
// All methods are called from the single solver Run goroutine; no locking is required.
type apiClient struct {
	c       *threef.APIClient
	sgnr    signer.Signer
	chainID *big.Int // operating chain; the grunt-api signing domain and the listOffers chainId query
	log     logr.Logger
}

func newAPIClient(baseURL string, sgnr signer.Signer, chainID *big.Int, timeout time.Duration, log logr.Logger) *apiClient {
	cfg := threef.NewConfiguration()
	cfg.Servers = threef.ServerConfigurations{{URL: baseURL}}
	// Bound every call; the generated client otherwise uses http.DefaultClient (no timeout) and a hung
	// request would stall the single solver loop, redemption scans included.
	cfg.HTTPClient = &http.Client{Timeout: timeout}
	return &apiClient{
		c:       threef.NewAPIClient(cfg),
		sgnr:    sgnr,
		chainID: chainID,
		log:     log,
	}
}

// listAuctions returns the current auctions, each carrying its EIP-712 domain (needed for signing); no auth required.
func (ac *apiClient) listAuctions(ctx context.Context) ([]threef.AuctionDto, error) {
	auctions, httpResp, err := ac.c.AuctionAPI.AuctionControllerListV1(ctx).Domain(true).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, apiErr("list auctions", httpResp, err)
	}
	return auctions, nil
}

// createOffer submits a signed offer and returns the remote offer ID assigned by 3F.
func (ac *apiClient) createOffer(ctx context.Context, dto threef.CreateOfferDto) (int64, error) {
	resp, httpResp, e := ac.c.OfferAPI.OfferControllerCreateV1(ctx).CreateOfferDto(dto).Execute()
	closeResp(httpResp)
	if e != nil {
		return 0, apiErr("create offer", httpResp, e)
	}
	if resp == nil {
		return 0, errors.New("3f api: create offer: empty response")
	}
	offerID, err := apiIDFromFloat(resp.Id, "create offer id")
	if err != nil {
		return 0, err
	}
	return offerID, nil
}

// listOffers returns the adapter's outstanding offers. Authenticated via a per-adapter EIP-712
// GetOffers signature in the Authorization: Bearer header — no API key required.
func (ac *apiClient) listOffers(ctx context.Context, adapter common.Address) ([]threef.OfferDto, error) {
	deadline := big.NewInt(time.Now().Add(getOffersDeadlineWindow).Unix())
	sig, err := ac.sgnr.SignHash(GetOffersDigest(adapter, deadline, ac.chainID))
	if err != nil {
		return nil, errors.Errorf("3f api: sign GetOffers: %w", err)
	}
	o, httpResp, e := ac.c.OfferAPI.OfferControllerGetV1(ctx).
		Maker(lowerAddr(adapter)).
		// chainId is the operating chain; the server rebuilds the grunt-api signing domain from it to
		// verify the signature and routes the EIP-1271 check to that chain.
		ChainId(float32(ac.chainID.Int64())).
		Deadline(deadline.String()).
		Authorization("Bearer 0x" + common.Bytes2Hex(sig)).
		Execute()
	closeResp(httpResp)
	if e != nil {
		return nil, apiErr("list offers", httpResp, e)
	}
	return o, nil
}

// getOfferByID returns one remote offer for adapter using the same signed authentication as listOffers.
func (ac *apiClient) getOfferByID(ctx context.Context, adapter common.Address, offerID int64) (*threef.OfferDto, error) {
	apiID, err := apiIDToFloat(offerID, "offer id")
	if err != nil {
		return nil, err
	}
	deadline := big.NewInt(time.Now().Add(getOffersDeadlineWindow).Unix())
	sig, err := ac.sgnr.SignHash(GetOffersDigest(adapter, deadline, ac.chainID))
	if err != nil {
		return nil, errors.Errorf("3f api: sign GetOffers: %w", err)
	}
	o, httpResp, e := ac.c.OfferAPI.OfferControllerGetByIdV1(ctx, apiID).
		Maker(lowerAddr(adapter)).
		ChainId(float32(ac.chainID.Int64())).
		Deadline(deadline.String()).
		Authorization("Bearer 0x" + common.Bytes2Hex(sig)).
		Execute()
	closeResp(httpResp)
	if e != nil {
		return nil, apiErr("get offer", httpResp, e)
	}
	return o, nil
}

// cancelOffer signs and submits a 3F cancel request. Solver policy does not call this yet; it is wired
// here so follow-up repricing/cancel flows can use the generated API surface without leaking it outward.
func (ac *apiClient) cancelOffer(ctx context.Context, adapter common.Address, offerID int64) (int64, string, error) {
	apiID, err := apiIDToFloat(offerID, "offer id")
	if err != nil {
		return 0, "", err
	}
	deadline := big.NewInt(time.Now().Add(getOffersDeadlineWindow).Unix())
	sig, err := ac.sgnr.SignHash(CancelOfferDigest(adapter, big.NewInt(offerID), deadline, ac.chainID))
	if err != nil {
		return 0, "", errors.Errorf("3f api: sign CancelOffer: %w", err)
	}
	dto := threef.NewCancelOfferDto(apiID, lowerAddr(adapter))
	dto.SetChainId(float32(ac.chainID.Int64()))
	dto.SetDeadline(deadline.String())
	dto.SetSignature("0x" + common.Bytes2Hex(sig))

	resp, httpResp, e := ac.c.OfferAPI.OfferControllerCancelV1(ctx).CancelOfferDto(*dto).Execute()
	closeResp(httpResp)
	if e != nil {
		return 0, "", apiErr("cancel offer", httpResp, e)
	}
	if resp == nil {
		return 0, "", errors.New("3f api: cancel offer: empty response")
	}
	id, err := apiIDFromFloat(resp.Id, "cancel offer id")
	if err != nil {
		return 0, "", err
	}
	return id, resp.Status, nil
}

// closeResp closes the response body. The generated client already closes it inside Execute; this
// satisfies bodyclose, which can't see across that call boundary.
func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// apiErr wraps a failed 3F call with its HTTP status and the server's response body — the client's own
// error is only the status line, but 3F returns the validation detail in the body.
func apiErr(what string, resp *http.Response, err error) error {
	var genErr *threef.GenericOpenAPIError
	if errors.As(err, &genErr) {
		if body := strings.TrimSpace(string(genErr.Body())); body != "" {
			return errors.Errorf("3f api: %s: %s: %s: %w", what, statusOf(resp), body, err)
		}
	}
	return errors.Errorf("3f api: %s: %s: %w", what, statusOf(resp), err)
}

// statusOf renders an HTTP response's status for error context ("no response" if there was none).
func statusOf(resp *http.Response) string {
	if resp == nil {
		return "no response"
	}
	return resp.Status
}

// lowerAddr renders an address as a lowercase hex string; the 3F API rejects checksummed addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }

func apiIDFromFloat(v float32, field string) (int64, error) {
	f := float64(v)
	if f < 1 || math.Trunc(f) != f {
		return 0, errors.Errorf("3f api: %s: invalid id %v", field, v)
	}
	return int64(v), nil
}

func apiIDToFloat(v int64, field string) (float32, error) {
	if v < 1 {
		return 0, errors.Errorf("3f api: %s: invalid id %d", field, v)
	}
	if v > maxExactFloat32Int {
		return 0, errors.Errorf("3f api: %s: id %d exceeds exact float32 range", field, v)
	}
	return float32(v), nil
}
