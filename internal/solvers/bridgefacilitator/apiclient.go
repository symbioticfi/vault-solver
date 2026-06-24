package bridgefacilitator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/go-errors/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/api/threef"
	"github.com/symbioticfi/vault-solver/internal/signer"
)

// keyRegenCooldown is the minimum spacing between generate-key calls. 3F rate-limits the endpoint
// ("API key was generated recently; try again later" → HTTP 429), so once we've just minted a key
// we must not immediately mint another. Crucially, a 401/403 right after issuing a key is an
// authorization problem with the facilitator/resource, NOT an expired key — regenerating would both
// trip the rate limit and revoke the working key, so within this window we surface the failure
// instead of regenerating. Legitimate mid-run expiry (hours later) is well outside the window.
const keyRegenCooldown = 2 * time.Minute

// apiClient wraps the generated 3F client. It injects the x-api-key header, lazily generates the
// key (EIP-712, signed by the facilitator), and reactively re-generates on a 401/403 — the 3F key
// has no documented TTL (a new generate-key revokes the prior key), so rather than assume a
// lifetime we refresh on demonstrated auth failure, rate-limited by keyRegenCooldown.
//
// All methods are called from the single solver Run goroutine, so the cached key needs no lock.
type apiClient struct {
	c            *threef.APIClient
	sgnr         signer.Signer
	facilitator  common.Address
	fallbackKey  string // operator-provided key (apiKeyEnv); used if self-generation is unavailable
	apiKey       string
	lastGenerate time.Time // when generate-key was last attempted, to honor 3F's rate limit
	log          logr.Logger
}

func newAPIClient(
	baseURL string, timeout time.Duration, sgnr signer.Signer, facilitator common.Address, fallbackKey string, log logr.Logger,
) (*apiClient, error) {
	if baseURL == "" {
		return nil, errors.New("3f api: base URL is required")
	}
	cfg := threef.NewConfiguration()
	cfg.Servers = threef.ServerConfigurations{{URL: baseURL}}
	// Bound every call: the generated client otherwise falls back to http.DefaultClient (no timeout),
	// so a hung request would stall the single solver loop, redemption scans included.
	cfg.HTTPClient = &http.Client{Timeout: timeout}
	ac := &apiClient{
		c:           threef.NewAPIClient(cfg),
		sgnr:        sgnr,
		facilitator: facilitator,
		fallbackKey: fallbackKey,
		log:         log,
	}
	if fallbackKey != "" {
		ac.setKey(fallbackKey, "env fallback")
	}
	return ac, nil
}

// setKey records the active x-api-key and logs a non-reversible fingerprint (not the key) so an
// operator can tell which key is active without the secret ever landing in logs.
func (ac *apiClient) setKey(key, source string) {
	ac.apiKey = key
	ac.log.V(1).Info("3F API key set", "source", source, "fingerprint", keyFingerprint(key))
}

// keyFingerprint is a short, non-reversible identifier for a secret, for log correlation only.
func keyFingerprint(key string) string {
	if key == "" {
		return "(empty)"
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:4])
}

// ensureKey makes sure a key is available, generating one if needed.
func (ac *apiClient) ensureKey(ctx context.Context) error {
	if ac.apiKey != "" {
		return nil
	}
	return ac.refreshKey(ctx)
}

// refreshKey generates a fresh key (revoking any prior one). If generation is unavailable (e.g. the
// facilitator isn't onboarded yet) and an operator key was supplied, it falls back to that.
//
// Within keyRegenCooldown of the last generate-key attempt it refuses to regenerate and returns an
// error: the existing key is the freshest 3F will issue, so a preceding 401/403 reflects an
// authorization problem (not expiry) and regenerating would only 429 and revoke the working key.
func (ac *apiClient) refreshKey(ctx context.Context) error {
	if !ac.lastGenerate.IsZero() && time.Since(ac.lastGenerate) < keyRegenCooldown {
		if ac.apiKey != "" {
			return errors.Errorf("3f api: key generated %s ago (within the %s regen cooldown); "+
				"auth failure is not key expiry — not regenerating",
				time.Since(ac.lastGenerate).Round(time.Second), keyRegenCooldown)
		}
		// No usable key and still cooling down (e.g. a prior process generated recently).
		if ac.fallbackKey != "" {
			ac.setKey(ac.fallbackKey, "env fallback")
			return nil
		}
		return errors.Errorf("3f api: generate-key on cooldown (last attempt %s ago) and no key available",
			time.Since(ac.lastGenerate).Round(time.Second))
	}
	key, err := ac.generate(ctx)
	if err != nil {
		if ac.fallbackKey != "" {
			ac.setKey(ac.fallbackKey, "env fallback")
			return nil
		}
		return err
	}
	ac.setKey(key, "generated")
	return nil
}

// generate signs the EIP-712 GenerateFacilitatorApiKey message and returns the issued key. It
// records the attempt time (arming keyRegenCooldown) even on failure, so a 429 can't be hammered.
func (ac *apiClient) generate(ctx context.Context) (string, error) {
	ac.lastGenerate = time.Now()
	deadline := big.NewInt(time.Now().Add(generateKeyDeadline).Unix())
	sig, err := ac.sgnr.SignHash(APIKeyDigest(ac.facilitator, deadline))
	if err != nil {
		return "", errors.Errorf("3f api: sign generate-key: %w", err)
	}
	dto := *threef.NewGenerateFacilitatorApiKeyDto(
		apiKeyDomainChainID,
		lowerAddr(ac.facilitator),
		deadline.String(),
		hexutil.Encode(sig),
	)
	resp, httpResp, err := ac.c.FacilitatorAPI.AdminControllerGenerateKeyV1(ctx).
		GenerateFacilitatorApiKeyDto(dto).Execute()
	closeResp(httpResp)
	if err != nil {
		return "", errors.Errorf("3f api: generate-key: %s: %w", statusOf(httpResp), err)
	}
	if resp == nil {
		return "", errors.Errorf("3f api: generate-key: empty response (%s)", statusOf(httpResp))
	}
	apiKey, ok := resp.GetApiKeyOk()
	if !ok || apiKey == nil || *apiKey == "" {
		return "", errors.Errorf("3f api: generate-key: response missing apiKey (%s)", statusOf(httpResp))
	}
	return *apiKey, nil
}

// withAuth runs an authed call, ensuring a key first and regenerating + retrying once on 401/403.
// `do` performs one attempt and returns the HTTP status of that attempt (so the auth-failure retry
// can trigger) plus any transport/decoding error.
func (ac *apiClient) withAuth(ctx context.Context, do func() (int, error)) error {
	if err := ac.ensureKey(ctx); err != nil {
		return err
	}
	status, err := do()
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		if rErr := ac.refreshKey(ctx); rErr != nil {
			return errors.Errorf("3f api: re-auth after %d: %w", status, rErr)
		}
		return wrapAttempt(do())
	}
	return err
}

// wrapAttempt collapses a (status, err) attempt into a single error (status is irrelevant once the
// retry has run — the error, if any, is what the caller cares about).
func wrapAttempt(_ int, err error) error { return err }

// listAuctions returns the current auctions, each carrying its EIP-712 domain (needed for signing).
// No auth needed here.
func (ac *apiClient) listAuctions(ctx context.Context) ([]threef.AuctionDto, error) {
	auctions, httpResp, err := ac.c.AuctionAPI.AuctionControllerListV1(ctx).Domain(true).Execute()
	closeResp(httpResp)
	if err != nil {
		return nil, errors.Errorf("3f api: list auctions: %s: %w", statusOf(httpResp), err)
	}
	return auctions, nil
}

// createOffer submits a signed offer.
func (ac *apiClient) createOffer(ctx context.Context, dto threef.CreateOfferDto) error {
	err := ac.withAuth(ctx, func() (int, error) {
		_, httpResp, e := ac.c.OfferAPI.OfferControllerCreateV1(ctx).
			XApiKey(ac.apiKey).CreateOfferDto(dto).Execute()
		closeResp(httpResp)
		if e != nil {
			return statusCode(httpResp), errors.Errorf("3f api: create offer: %s: %w", statusOf(httpResp), e)
		}
		return statusCode(httpResp), nil
	})
	if err != nil {
		return errors.Errorf("3f api: create offer: %w", err)
	}
	return nil
}

// listOffers returns the facilitator's offers. Used at startup to rebuild the offer-dedup cache so a
// restart doesn't re-offer on auctions we already have live offers for.
//
// On the x-api-key path the API requires `maker` to be the facilitator's own broker address (not the
// adapter); it then returns offers under that address AND under the facilitator's configured
// offer-address — which is our adapter (see ensureOfferAddress). So querying by the facilitator
// surfaces our adapter's offers. (Querying maker=adapter here returns 403: that scope needs an
// EIP-712 GetOffers signature instead of the api key.)
func (ac *apiClient) listOffers(ctx context.Context) ([]threef.OfferDto, error) {
	makerLower := lowerAddr(ac.facilitator)
	var offers []threef.OfferDto
	err := ac.withAuth(ctx, func() (int, error) {
		o, httpResp, e := ac.c.OfferAPI.OfferControllerGetV1(ctx).
			Maker(makerLower).XApiKey(ac.apiKey).Execute()
		closeResp(httpResp)
		if e != nil {
			return statusCode(httpResp), errors.Errorf("3f api: list offers: %s: %w", statusOf(httpResp), e)
		}
		offers = o
		return statusCode(httpResp), nil
	})
	if err != nil {
		return nil, errors.Errorf("3f api: list offers: %w", err)
	}
	return offers, nil
}

// offerAddress returns the facilitator's currently-registered offer (maker) address, or the zero
// address if none is set.
func (ac *apiClient) offerAddress(ctx context.Context) (common.Address, error) {
	var addr common.Address
	err := ac.withAuth(ctx, func() (int, error) {
		resp, httpResp, e := ac.c.FacilitatorAPI.AdminControllerGetFacilitatorOfferAddressV1(ctx).
			XApiKey(ac.apiKey).Execute()
		closeResp(httpResp)
		if e != nil {
			return statusCode(httpResp), errors.Errorf("3f api: get offer-address: %s: %w", statusOf(httpResp), e)
		}
		if s, ok := resp.GetOfferAddressOk(); ok && s != nil && common.IsHexAddress(*s) {
			addr = common.HexToAddress(*s)
		}
		return statusCode(httpResp), nil
	})
	if err != nil {
		return common.Address{}, errors.Errorf("3f api: get offer-address: %w", err)
	}
	return addr, nil
}

// setOfferAddress registers `addr` as the facilitator's offer (maker) address.
func (ac *apiClient) setOfferAddress(ctx context.Context, addr common.Address) error {
	dto := *threef.NewSetFacilitatorOfferAddressDto(lowerAddr(addr))
	err := ac.withAuth(ctx, func() (int, error) {
		_, httpResp, e := ac.c.FacilitatorAPI.AdminControllerSetFacilitatorOfferAddressV1(ctx).
			XApiKey(ac.apiKey).SetFacilitatorOfferAddressDto(dto).Execute()
		closeResp(httpResp)
		if e != nil {
			return statusCode(httpResp), errors.Errorf("3f api: set offer-address: %s: %w", statusOf(httpResp), e)
		}
		return statusCode(httpResp), nil
	})
	if err != nil {
		return errors.Errorf("3f api: set offer-address: %w", err)
	}
	return nil
}

// closeResp closes the HTTP response body. The generated client already reads the body fully and
// closes it inside Execute, so this is a harmless no-op that satisfies the "body must be closed"
// contract without a lint suppression (bodyclose can't see across the Execute call boundary).
func closeResp(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// statusCode returns the HTTP status code of resp, or 0 if resp is nil (e.g. a transport error
// before any response). The auth-retry logic keys off this, so a nil response must not look like
// a 401/403.
func statusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
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
