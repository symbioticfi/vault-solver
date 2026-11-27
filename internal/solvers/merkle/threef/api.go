package threef

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

const (
	maxResponseBytes int64 = 2 << 20
	maxRows                = 4096
	requestTimeout         = 30 * time.Second
	// exactMaxRateKey preserves the upstream decimal before the generated float32 conversion.
	exactMaxRateKey   = "exactMaxRate"
	exactRateBasisKey = "exactRateBasisSeconds"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type apiConfig struct {
	URL       string         `json:"url"`
	APIKeyEnv string         `json:"apiKeyEnv"`
	Broker    common.Address `json:"broker"`
}

// apiClient is safe for concurrent calls; no API credentials are retained in it.
type apiClient struct {
	c      *threef.APIClient
	broker common.Address
}

func validateAPIConfig(cfg apiConfig) error {
	if err := validateURL(cfg.URL); err != nil {
		return err
	}
	if !envName.MatchString(cfg.APIKeyEnv) {
		return errors.New("3f apiKeyEnv is not an environment variable name")
	}
	if cfg.Broker == (common.Address{}) {
		return errors.New("3f broker must be a nonzero address")
	}
	return nil
}

// validateURL names the failed check without echoing the URL, which may carry credentials.
func validateURL(baseURL string) error {
	u, err := url.Parse(baseURL)
	var problem string
	switch {
	case err != nil:
		problem = "does not parse"
	case u.Scheme != "https" && u.Scheme != "http":
		problem = "scheme is not http or https"
	case u.Host == "":
		problem = "has no host"
	case u.User != nil:
		problem = "contains credentials"
	case u.RawQuery != "" || u.ForceQuery:
		problem = "contains a query"
	case u.Fragment != "":
		problem = "contains a fragment"
	case u.Path != "" && u.Path != "/":
		problem = "has a path"
	default:
		return nil
	}
	return errors.Errorf("3f URL %s; it must be an http(s) origin", problem)
}

func newClient(cfg apiConfig) (*apiClient, error) {
	if err := validateAPIConfig(cfg); err != nil {
		return nil, err
	}
	// Fail at startup instead of on every request; the key is re-read per request, never kept.
	if _, err := apiKey(cfg.APIKeyEnv); err != nil {
		return nil, err
	}
	c := threef.NewConfiguration()
	c.Servers = threef.ServerConfigurations{{URL: strings.TrimRight(cfg.URL, "/") + c.Servers[0].URL}}
	c.HTTPClient = &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: boundedTransport{keyEnv: cfg.APIKeyEnv}}
	return &apiClient{c: threef.NewAPIClient(c), broker: cfg.Broker}, nil
}

func (c *apiClient) Broker() common.Address { return c.broker }

func (c *apiClient) Identity(ctx context.Context, account common.Address) error {
	identity, response, err := c.c.FacilitatorAPI.AdminControllerGetFacilitatorOfferAddressV1(ctx).XApiKey("").Execute()
	closeResponse(response)
	if err != nil {
		return apiError(ctx, "read identity", response, err)
	}
	switch {
	case account == (common.Address{}):
		return errors.New("3f executor account is the zero address")
	case identity == nil:
		return errors.New("3f identity response is empty")
	case !matches(identity.Facilitator, c.broker):
		return errors.Errorf("3f identity facilitator %.66q differs from configured broker %s", identity.Facilitator, c.broker.Hex())
	case !matches(identity.OfferAddress, account):
		return errors.Errorf("3f identity offerAddress %.66q differs from executor account %s", identity.OfferAddress, account.Hex())
	}
	makers, response, err := c.c.FacilitatorAPI.AdminControllerListMakersV1(ctx).XApiKey("").Execute()
	closeResponse(response)
	if err != nil {
		return apiError(ctx, "read enrolled makers", response, err)
	}
	var found bool
	for i, maker := range makers {
		if !validAddress(maker.Maker) {
			return errors.Errorf("3f enrolled maker %d address %.66q is not a nonzero address", i, maker.Maker)
		}
		if matches(maker.Maker, account) {
			switch {
			case found:
				return errors.Errorf("3f maker %s is enrolled more than once", account.Hex())
			case !maker.SubmissionsEnabled:
				return errors.Errorf("3f maker %s has submissions disabled", account.Hex())
			case !maker.IsDefault:
				return errors.Errorf("3f maker %s is not the default maker", account.Hex())
			}
			found = true
		}
	}
	if !found {
		return errors.Errorf("3f account %s is not an enrolled maker", account.Hex())
	}
	return nil
}

func (c *apiClient) Offers(ctx context.Context) ([]threef.OfferDto, error) {
	rows, response, err := c.c.OfferAPI.OfferControllerGetV1(ctx).Maker(lower(c.broker)).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "list offers", response, err)
	}
	if rows == nil {
		return nil, errors.New("3f offers response is empty")
	}
	if len(rows) > maxRows {
		return nil, errors.Errorf("3f offers response has %d rows, above the %d-row bound", len(rows), maxRows)
	}
	for i, row := range rows {
		if err = validateOffer(row); err != nil {
			return nil, errors.Errorf("3f offers response row %d: %w", i, err)
		}
	}
	return rows, nil
}

func (c *apiClient) Auctions(ctx context.Context) ([]threef.AuctionDto, error) {
	rows, response, err := c.c.AuctionAPI.AuctionControllerListV1(ctx).Scope("all").Execute()
	defer closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "list auctions", response, err)
	}
	// The generated response keeps a fresh copy of its body. Preserve exact rate numbers from it.
	switch {
	case rows == nil || response == nil:
		return nil, errors.New("3f auctions response is empty")
	case len(rows) > maxRows:
		return nil, errors.Errorf("3f auctions response has %d rows, above the %d-row bound", len(rows), maxRows)
	}
	var raw []map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&raw); err != nil {
		return nil, errors.Errorf("3f auctions response is not an array of objects: %w", err)
	}
	if len(raw) != len(rows) {
		return nil, errors.Errorf("3f auctions response has %d raw rows but %d decoded rows", len(raw), len(rows))
	}
	for i := range rows {
		if err := validateID(rows[i].Id); err != nil {
			return nil, errors.Errorf("3f auctions response row %d: %w", i, err)
		}
		switch {
		case !validAddress(rows[i].RequestId):
			return nil, errors.Errorf("3f auction %v: requestId %.66q is not a nonzero address", rows[i].Id, rows[i].RequestId)
		case rows[i].Status == "":
			return nil, errors.Errorf("3f auction %v has no status", rows[i].Id)
		case !rows[i].AmountRequested.IsSet():
			return nil, errors.Errorf("3f auction %v has no amountRequested", rows[i].Id)
		case !rows[i].MaxRate.IsSet():
			return nil, errors.Errorf("3f auction %v has no maxRate", rows[i].Id)
		}
		if amount := rows[i].AmountRequested.Get(); amount != nil {
			if _, err := uintAmount(*amount); err != nil {
				return nil, errors.Errorf("3f auction %v amountRequested: %w", rows[i].Id, err)
			}
		}
		if rows[i].AdditionalProperties == nil {
			rows[i].AdditionalProperties = make(map[string]any)
		}
		delete(rows[i].AdditionalProperties, exactMaxRateKey)
		if value := raw[i]["maxRate"]; len(value) > 0 && string(value) != "null" {
			var number any
			d := json.NewDecoder(bytes.NewReader(value))
			d.UseNumber()
			if err := d.Decode(&number); err != nil {
				return nil, errors.Errorf("3f auction %v maxRate does not decode: %w", rows[i].Id, err)
			}
			n, ok := number.(json.Number)
			if !ok {
				return nil, errors.Errorf("3f auction %v maxRate is a JSON %T, not a number", rows[i].Id, number)
			}
			rows[i].AdditionalProperties[exactMaxRateKey] = string(n)
		}
		delete(rows[i].AdditionalProperties, exactRateBasisKey)
		if value := raw[i]["rateBasisSeconds"]; len(value) > 0 && string(value) != "null" {
			rows[i].AdditionalProperties[exactRateBasisKey] = string(value)
		}
	}
	return rows, nil
}

// Create sends exactly one request. Its caller must retain the immutable DTO in RAM before invoking it.
func (c *apiClient) Create(ctx context.Context, dto threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error) {
	if problem := unsignedOfferProblem(dto, c.broker); problem != "" {
		return nil, errors.Errorf("3f unsigned PUSH offer on auction %v: %s", dto.AuctionId, problem)
	}
	dto.Maker = lower(c.broker)
	result, response, err := c.c.OfferAPI.OfferControllerCreateV1(ctx).CreateOfferDto(dto).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "create offer", response, err)
	}
	if result == nil {
		return nil, errors.New("3f create offer response is empty")
	}
	if err := validateID(result.Id); err != nil {
		return nil, errors.Errorf("3f created offer: %w", err)
	}
	return result, nil
}

// unsignedOfferProblem names the first field that keeps dto from being an unsigned PUSH offer by
// broker, "" when it is one.
func unsignedOfferProblem(dto threef.CreateOfferDto, broker common.Address) string {
	if nonce, err := uintAmount(dto.Nonce); err != nil || !nonce.IsInt64() {
		return fmt.Sprintf("nonce %q is not a nonnegative int64", dto.Nonce)
	}
	if err := validateID(dto.AuctionId); err != nil {
		return err.Error()
	}
	switch {
	case !matches(dto.Maker, broker):
		return fmt.Sprintf("maker %q is not broker %s", dto.Maker, broker.Hex())
	case !validUint(dto.Amount, true):
		return fmt.Sprintf("amount %q is not a positive uint256", dto.Amount)
	case !validUint(dto.ExpectedReturn, false):
		return fmt.Sprintf("expectedReturn %q is not a uint256", dto.ExpectedReturn)
	case !validUint(dto.Expiration, true):
		return fmt.Sprintf("expiration %q is not a positive uint256", dto.Expiration)
	case dto.UseCallback:
		return "useCallback is set"
	case dto.ChainId != nil:
		return "chainId is set"
	case dto.ExecutionMode != nil:
		return "executionMode is set"
	case dto.Signature != nil:
		return "signature is set"
	case dto.PushAuthorizationSignature != nil:
		return "pushAuthorizationSignature is set"
	case dto.AuthorizationDeadline != nil:
		return "authorizationDeadline is set"
	case len(dto.AdditionalProperties) > 0:
		return fmt.Sprintf("has %d additional properties", len(dto.AdditionalProperties))
	}
	return ""
}

// Cancel rechecks selection state; the API's own atomic guard handles acceptance racing the POST.
func (c *apiClient) Cancel(ctx context.Context, id uint64) (*threef.CancelOfferResponseDto, error) {
	if id == 0 || id >= 1<<24 {
		return nil, errors.Errorf("3f offer ID %d must be an exact positive integer below 16777216", id)
	}
	row, response, err := c.c.OfferAPI.OfferControllerGetByIdV1(ctx, float32(id)).Maker(lower(c.broker)).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "read offer before cancellation", response, err)
	}
	if row == nil {
		return nil, errors.Errorf("3f offer %d read before cancellation is empty", id)
	}
	if err := validateOffer(*row); err != nil {
		return nil, errors.Errorf("3f offer %d read before cancellation: %w", id, err)
	}
	switch {
	case row.Id != float32(id):
		return nil, errors.Errorf("3f offer %d read before cancellation returned offer %v", id, row.Id)
	case row.Status != "SUBMITTED":
		return nil, errors.Errorf("3f offer %d has status %.32q; only SUBMITTED offers can be cancelled", id, row.Status)
	case row.ExecutionMode != "PUSH":
		return nil, errors.Errorf("3f offer %d has execution mode %.32q; only PUSH offers can be cancelled", id, row.ExecutionMode)
	}
	dto := *threef.NewCancelOfferDto(float32(id), lower(c.broker))
	result, response, err := c.c.OfferAPI.OfferControllerCancelV1(ctx).CancelOfferDto(dto).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "cancel offer", response, err)
	}
	switch {
	case result == nil:
		return nil, errors.Errorf("3f cancel offer %d response is empty", id)
	case result.Id != float32(id):
		return nil, errors.Errorf("3f cancel offer %d response names offer %v", id, result.Id)
	case result.Status != "CANCELLED":
		return nil, errors.Errorf("3f cancel offer %d response has status %.32q, want CANCELLED", id, result.Status)
	}
	return result, nil
}

// validateID bounds the generated float32 representation to its exact consecutive integer range.
func validateID(id float32) error {
	if id <= 0 || id >= 1<<24 || float64(id) != math.Trunc(float64(id)) || math.IsNaN(float64(id)) {
		return errors.Errorf("3f ID %v must be an exact positive integer below 16777216", id)
	}
	return nil
}

func validateOffer(row threef.OfferDto) error {
	if err := validateID(row.Id); err != nil {
		return errors.Errorf("3f offer id: %w", err)
	}
	if err := validateID(row.AuctionId); err != nil {
		return errors.Errorf("3f offer %v auctionId: %w", row.Id, err)
	}
	switch {
	case !validAddress(row.Maker):
		return errors.Errorf("3f offer %v: maker %.66q is not a nonzero address", row.Id, row.Maker)
	case !validAddress(row.RequestId):
		return errors.Errorf("3f offer %v: requestId %.66q is not a nonzero address", row.Id, row.RequestId)
	}
	switch row.Status {
	case "SUBMITTED", "ACCEPTED", "AWAITING_MINT", "CANCELLED", "SUCCEEDED", "FAILED", "NOT_ACCEPTED":
	default:
		return errors.Errorf("3f offer %v has unknown status %.32q", row.Id, row.Status)
	}
	switch {
	case row.ExecutionMode != "PUSH" && row.ExecutionMode != "PULL":
		return errors.Errorf("3f offer %v has unknown execution mode %.32q", row.Id, row.ExecutionMode)
	case row.ExecutionMode == "PUSH" && row.Signature.Get() != nil:
		return errors.Errorf("3f PUSH offer %v carries an executable signature", row.Id)
	case !row.Signature.IsSet():
		return errors.Errorf("3f offer %v has no signature field", row.Id)
	case !row.MintDeadline.IsSet():
		return errors.Errorf("3f offer %v has no mintDeadline field", row.Id)
	}
	for _, field := range []struct{ name, value string }{
		{"amount", row.Amount}, {"expectedReturn", row.ExpectedReturn}, {"filledAmount", row.FilledAmount},
		{"filledExpectedReturn", row.FilledExpectedReturn}, {"pendingAmount", row.PendingAmount},
		{"pendingExpectedReturn", row.PendingExpectedReturn}, {"nonce", row.Nonce}, {"expiration", row.Expiration},
	} {
		if _, err := uintAmount(field.value); err != nil {
			return errors.Errorf("3f offer %v %s: %w", row.Id, field.name, err)
		}
	}
	if deadline := row.MintDeadline.Get(); deadline != nil {
		if _, err := time.Parse(time.RFC3339Nano, *deadline); err != nil {
			return errors.Errorf("3f offer %v mintDeadline %.40q is not RFC 3339", row.Id, *deadline)
		}
	}
	return nil
}

func validUint(value string, positive bool) bool {
	n, err := uintAmount(value)
	return err == nil && (!positive || n.Sign() > 0)
}

func validAddress(value string) bool {
	return common.IsHexAddress(value) && common.HexToAddress(value) != (common.Address{})
}
func matches(value string, expected common.Address) bool {
	return validAddress(value) && common.HexToAddress(value) == expected
}
func lower(value common.Address) string { return strings.ToLower(value.Hex()) }

func apiKey(env string) (string, error) {
	key := os.Getenv(env)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return "", errors.Errorf("3f API key in %s is missing or invalid", env)
	}
	return key, nil
}

type boundedTransport struct{ keyEnv string }

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if t.keyEnv != "" {
		key, err := apiKey(t.keyEnv)
		if err != nil {
			return nil, err
		}
		// Generated methods may insert a noncanonical lowercase empty header.
		for name := range clone.Header {
			if strings.EqualFold(name, "x-api-key") {
				delete(clone.Header, name)
			}
		}
		clone.Header.Set("x-api-key", key)
	}
	// The transport error and URL can carry credentials; the method and path are our own.
	response, err := http.DefaultTransport.RoundTrip(clone)
	if err != nil {
		return nil, errors.Errorf("3f HTTP transport failed for %s %s", req.Method, req.URL.Path)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, errors.Errorf("3f %s %s response read failed after %d bytes", req.Method, req.URL.Path, len(body))
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, errors.Errorf("3f %s %s response exceeds %d bytes", req.Method, req.URL.Path, maxResponseBytes)
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err = checkJSON(body); err != nil {
			return nil, err
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

type apiStatusError struct {
	status    int
	operation string
}

func (e *apiStatusError) Error() string {
	return "3f " + e.operation + ": HTTP status " + strconv.Itoa(e.status)
}

func apiError(ctx context.Context, operation string, response *http.Response, err error) error {
	if ctx.Err() != nil {
		return errors.Errorf("3f %s: %w", operation, ctx.Err())
	}
	if response == nil {
		// Keep the cause but drop url.Error's request URL from the message.
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return errors.Errorf("3f %s failed: %w", operation, err)
	}
	return &apiStatusError{status: response.StatusCode, operation: operation}
}

func definiteRejection(err error) bool {
	var failure *apiStatusError
	if !errors.As(err, &failure) {
		return false
	}
	switch failure.status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusMethodNotAllowed, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity:
		return true
	default:
		return false
	}
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
