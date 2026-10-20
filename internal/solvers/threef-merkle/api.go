package threefmerkle

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/api/threef"
)

const (
	maxResponseBytes int64 = 2 << 20
	requestTimeout         = 30 * time.Second
	// exactMaxRateKey preserves the upstream decimal before the generated float32 conversion.
	exactMaxRateKey = "exactMaxRate"
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
	if !envName.MatchString(cfg.APIKeyEnv) || cfg.Broker == (common.Address{}) {
		return errors.New("3f API key environment name and nonzero broker are required")
	}
	return nil
}

func validateURL(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return errors.New("3f URL must be an http(s) origin without credentials, query or fragment")
	}
	return nil
}

func newClient(cfg apiConfig) (*apiClient, error) {
	if err := validateAPIConfig(cfg); err != nil {
		return nil, err
	}
	c := threef.NewConfiguration()
	c.Servers = threef.ServerConfigurations{{URL: strings.TrimRight(cfg.URL, "/")}}
	c.HTTPClient = &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Transport: boundedTransport{keyEnv: cfg.APIKeyEnv}}
	return &apiClient{c: threef.NewAPIClient(c), broker: cfg.Broker}, nil
}

func (c *apiClient) Broker() common.Address { return c.broker }

func (c *apiClient) Identity(ctx context.Context, account common.Address) error {
	identity, response, err := c.c.FacilitatorAPI.AdminControllerGetFacilitatorOfferAddressV1(ctx).XApiKey("").Execute()
	closeResponse(response)
	if err != nil {
		return apiError(ctx, "read identity", response)
	}
	if identity == nil || !matches(identity.Facilitator, c.broker) || !matches(identity.OfferAddress, account) || account == (common.Address{}) {
		return errors.New("3f broker or default maker mismatch")
	}
	makers, response, err := c.c.FacilitatorAPI.AdminControllerListMakersV1(ctx).XApiKey("").Execute()
	closeResponse(response)
	if err != nil {
		return apiError(ctx, "read enrolled makers", response)
	}
	var found bool
	for _, maker := range makers {
		if !validAddress(maker.Maker) {
			return errors.New("3f invalid enrolled maker")
		}
		if matches(maker.Maker, account) {
			if found || !maker.SubmissionsEnabled || !maker.IsDefault {
				return errors.New("3f default maker is not uniquely enabled")
			}
			found = true
		}
	}
	if !found {
		return errors.New("3f default maker is not enrolled")
	}
	return nil
}

func (c *apiClient) Offers(ctx context.Context) ([]threef.OfferDto, error) {
	rows, response, err := c.c.OfferAPI.OfferControllerGetV1(ctx).Maker(lower(c.broker)).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "list offers", response)
	}
	if rows == nil || len(rows) > 4096 {
		return nil, errors.New("3f invalid or oversized offers response")
	}
	for _, row := range rows {
		if err = validateOffer(row); err != nil {
			return nil, err
		}
	}
	return rows, nil
}

func (c *apiClient) Auctions(ctx context.Context) ([]threef.AuctionDto, error) {
	rows, response, err := c.c.AuctionAPI.AuctionControllerListV1(ctx).Scope("all").Execute()
	defer closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "list auctions", response)
	}
	// The generated response keeps a fresh copy of its body. Read the exact rate from that copy.
	var raw []map[string]json.RawMessage
	if rows == nil || len(rows) > 4096 || response == nil || json.NewDecoder(response.Body).Decode(&raw) != nil || len(raw) != len(rows) {
		return nil, errors.New("3f invalid auctions response")
	}
	for i := range rows {
		if validateID(rows[i].Id) != nil || !validAddress(rows[i].RequestId) || rows[i].Status == "" {
			return nil, errors.New("3f invalid auction identity")
		}
		if !rows[i].AmountRequested.IsSet() || !rows[i].MaxRate.IsSet() {
			return nil, errors.New("3f missing auction amount or rate")
		}
		if amount := rows[i].AmountRequested.Get(); amount != nil && !validUint(*amount, false) {
			return nil, errors.New("3f invalid auction amount")
		}
		if rows[i].AdditionalProperties == nil {
			rows[i].AdditionalProperties = make(map[string]any)
		}
		delete(rows[i].AdditionalProperties, exactMaxRateKey)
		if value := raw[i]["maxRate"]; len(value) > 0 && string(value) != "null" {
			var number any
			d := json.NewDecoder(bytes.NewReader(value))
			d.UseNumber()
			if d.Decode(&number) != nil {
				return nil, errors.New("3f invalid auction rate")
			}
			n, ok := number.(json.Number)
			if !ok {
				return nil, errors.New("3f invalid auction rate")
			}
			rows[i].AdditionalProperties[exactMaxRateKey] = string(n)
		}
	}
	return rows, nil
}

// Create sends exactly one request. Its caller must journal the immutable DTO before invoking it.
func (c *apiClient) Create(ctx context.Context, dto threef.CreateOfferDto) (*threef.CreateOfferResponseDto, error) {
	if validateID(dto.AuctionId) != nil || !matches(dto.Maker, c.broker) || !validUint(dto.Amount, true) || !validUint(dto.ExpectedReturn, false) || !validUint(dto.Nonce, false) || !validUint(dto.Expiration, true) || dto.UseCallback || dto.ChainId != nil || dto.ExecutionMode != nil || dto.Signature != nil || dto.PushAuthorizationSignature != nil || dto.AuthorizationDeadline != nil || len(dto.AdditionalProperties) > 0 {
		return nil, errors.New("3f invalid unsigned PUSH offer")
	}
	dto.Maker = lower(c.broker)
	result, response, err := c.c.OfferAPI.OfferControllerCreateV1(ctx).CreateOfferDto(dto).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "create offer", response)
	}
	if result == nil || validateID(result.Id) != nil {
		return nil, errors.New("3f invalid created offer ID")
	}
	return result, nil
}

// Cancel rechecks selection state; the API's own atomic guard handles acceptance racing the POST.
func (c *apiClient) Cancel(ctx context.Context, id uint64) (*threef.CancelOfferResponseDto, error) {
	if id == 0 || id >= 1<<24 {
		return nil, errors.New("3f offer ID must be an exact positive integer below 16777216")
	}
	row, response, err := c.c.OfferAPI.OfferControllerGetByIdV1(ctx, float32(id)).Maker(lower(c.broker)).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "read offer before cancellation", response)
	}
	if row == nil || validateOffer(*row) != nil || row.Id != float32(id) || row.Status != "SUBMITTED" || row.ExecutionMode != "PUSH" {
		return nil, errors.New("3f only unaccepted PUSH offers can be cancelled")
	}
	dto := *threef.NewCancelOfferDto(float32(id), lower(c.broker))
	result, response, err := c.c.OfferAPI.OfferControllerCancelV1(ctx).CancelOfferDto(dto).Execute()
	closeResponse(response)
	if err != nil {
		return nil, apiError(ctx, "cancel offer", response)
	}
	if result == nil || result.Id != float32(id) || result.Status != "CANCELLED" {
		return nil, errors.New("3f invalid cancelled offer response")
	}
	return result, nil
}

// validateID bounds the generated float32 representation to its exact consecutive integer range.
func validateID(id float32) error {
	if id <= 0 || id >= 1<<24 || float64(id) != math.Trunc(float64(id)) || math.IsNaN(float64(id)) {
		return errors.New("3f ID must be an exact positive integer below 16777216")
	}
	return nil
}

func validateOffer(row threef.OfferDto) error {
	if validateID(row.Id) != nil || validateID(row.AuctionId) != nil || !validAddress(row.Maker) || !validAddress(row.RequestId) {
		return errors.New("3f invalid offer identity")
	}
	switch row.Status {
	case "SUBMITTED", "ACCEPTED", "AWAITING_MINT", "CANCELLED", "SUCCEEDED", "FAILED", "NOT_ACCEPTED":
	default:
		return errors.New("3f unknown offer status")
	}
	if row.ExecutionMode != "PUSH" && row.ExecutionMode != "PULL" {
		return errors.New("3f unknown offer execution mode")
	}
	if row.ExecutionMode == "PUSH" && row.Signature.Get() != nil {
		return errors.New("3f PUSH offer contains executable signature")
	}
	if !row.Signature.IsSet() || !row.MintDeadline.IsSet() {
		return errors.New("3f missing offer signature or mint deadline")
	}
	for _, value := range []string{row.Amount, row.ExpectedReturn, row.FilledAmount, row.FilledExpectedReturn, row.PendingAmount, row.PendingExpectedReturn, row.Nonce, row.Expiration} {
		if !validUint(value, false) {
			return errors.New("3f invalid offer amount or nonce")
		}
	}
	if deadline := row.MintDeadline.Get(); deadline != nil {
		if _, err := time.Parse(time.RFC3339Nano, *deadline); err != nil {
			return errors.New("3f invalid mint deadline")
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

type boundedTransport struct{ keyEnv string }

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	if t.keyEnv != "" {
		key := os.Getenv(t.keyEnv)
		if key == "" || strings.ContainsAny(key, "\r\n") {
			return nil, errors.New("3f API key is missing or invalid")
		}
		clone.Header.Set("x-api-key", key)
	}
	response, err := http.DefaultTransport.RoundTrip(clone)
	if err != nil {
		return nil, errors.New("3f HTTP transport failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil || int64(len(body)) > maxResponseBytes {
		return nil, errors.New("3f response read failed or exceeded bound")
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		if err = checkJSON(body); err != nil {
			return nil, err
		}
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func apiError(ctx context.Context, operation string, response *http.Response) error {
	if ctx.Err() != nil {
		return errors.Errorf("3f %s: %w", operation, ctx.Err())
	}
	if response != nil {
		return errors.Errorf("3f %s: HTTP status %d", operation, response.StatusCode)
	}
	return errors.Errorf("3f %s failed", operation)
}

func closeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}
