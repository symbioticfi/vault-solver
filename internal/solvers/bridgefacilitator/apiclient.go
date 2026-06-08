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
	c            *threef.ClientWithResponses
	sgnr         signer.Signer
	facilitator  common.Address
	fallbackKey  string // operator-provided key (apiKeyEnv); used if self-generation is unavailable
	apiKey       string
	lastGenerate time.Time // when generate-key was last attempted, to honor 3F's rate limit
	log          logr.Logger
}

func newAPIClient(
	baseURL string, sgnr signer.Signer, facilitator common.Address, fallbackKey string, log logr.Logger,
) (*apiClient, error) {
	ac := &apiClient{sgnr: sgnr, facilitator: facilitator, fallbackKey: fallbackKey, log: log}
	c, err := threef.NewClientWithResponses(baseURL, threef.WithRequestEditorFn(ac.injectAPIKey))
	if err != nil {
		return nil, errors.Errorf("3f api: new client: %w", err)
	}
	ac.c = c
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

func (ac *apiClient) injectAPIKey(_ context.Context, req *http.Request) error {
	if ac.apiKey != "" {
		req.Header.Set("x-api-key", ac.apiKey)
	}
	return nil
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
	resp, err := ac.c.AdminControllerGenerateKeyV1WithResponse(ctx, threef.GenerateFacilitatorApiKeyDto{
		ChainId:     apiKeyDomainChainID,
		Facilitator: lowerAddr(ac.facilitator),
		Deadline:    deadline.String(),
		Signature:   hexutil.Encode(sig),
	})
	if err != nil {
		return "", errors.Errorf("3f api: generate-key: %w", err)
	}
	if resp.JSON201 == nil {
		return "", errors.Errorf("3f api: generate-key: status %s: %s", resp.Status(), string(resp.Body))
	}
	return resp.JSON201.ApiKey, nil
}

// withAuth runs an authed call, ensuring a key first and regenerating + retrying once on 401/403.
func (ac *apiClient) withAuth(ctx context.Context, do func() (int, error)) error {
	if err := ac.ensureKey(ctx); err != nil {
		return err
	}
	status, err := do()
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		if rErr := ac.refreshKey(ctx); rErr != nil {
			return errors.Errorf("3f api: re-auth after %d: %w", status, rErr)
		}
		if _, err = do(); err != nil {
			return err
		}
	}
	return nil
}

// listAuctions returns the current auctions, each carrying its EIP-712 domain (needed for signing). No auth needed here.
func (ac *apiClient) listAuctions(ctx context.Context) ([]threef.AuctionDto, error) {
	withDomain := true
	params := &threef.AuctionControllerListV1Params{Domain: &withDomain}
	resp, err := ac.c.AuctionControllerListV1WithResponse(ctx, params)
	if err != nil {
		return nil, errors.Errorf("3f api: list auctions: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, errors.Errorf("3f api: list auctions: unexpected status %s", resp.Status())
	}
	return *resp.JSON200, nil
}

// createOffer submits a signed offer.
func (ac *apiClient) createOffer(ctx context.Context, dto threef.CreateOfferDto) error {
	var resp *threef.OfferControllerCreateV1Response
	err := ac.withAuth(ctx, func() (int, error) {
		r, e := ac.c.OfferControllerCreateV1WithResponse(ctx, nil, dto)
		if e != nil {
			return 0, e
		}
		resp = r
		return r.StatusCode(), nil
	})
	if err != nil {
		return errors.Errorf("3f api: create offer: %w", err)
	}
	if resp.JSON201 == nil {
		return errors.Errorf("3f api: create offer: unexpected status %s: %s", resp.Status(), string(resp.Body))
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
	var resp *threef.OfferControllerGetV1Response
	err := ac.withAuth(ctx, func() (int, error) {
		r, e := ac.c.OfferControllerGetV1WithResponse(ctx, &threef.OfferControllerGetV1Params{Maker: makerLower})
		if e != nil {
			return 0, e
		}
		resp = r
		return r.StatusCode(), nil
	})
	if err != nil {
		return nil, errors.Errorf("3f api: list offers: %w", err)
	}
	if resp.JSON200 == nil {
		return nil, errors.Errorf("3f api: list offers: unexpected status %s: %s", resp.Status(), string(resp.Body))
	}
	return *resp.JSON200, nil
}

// offerAddress returns the facilitator's currently-registered offer (maker) address, or the zero
// address if none is set.
func (ac *apiClient) offerAddress(ctx context.Context) (common.Address, error) {
	var resp *threef.AdminControllerGetFacilitatorOfferAddressV1Response
	err := ac.withAuth(ctx, func() (int, error) {
		r, e := ac.c.AdminControllerGetFacilitatorOfferAddressV1WithResponse(ctx, nil)
		if e != nil {
			return 0, e
		}
		resp = r
		return r.StatusCode(), nil
	})
	if err != nil {
		return common.Address{}, errors.Errorf("3f api: get offer-address: %w", err)
	}
	if resp.JSON200 == nil || !common.IsHexAddress(resp.JSON200.OfferAddress) {
		return common.Address{}, nil // none set yet
	}
	return common.HexToAddress(resp.JSON200.OfferAddress), nil
}

// setOfferAddress registers `addr` as the facilitator's offer (maker) address.
func (ac *apiClient) setOfferAddress(ctx context.Context, addr common.Address) error {
	var resp *threef.AdminControllerSetFacilitatorOfferAddressV1Response
	err := ac.withAuth(ctx, func() (int, error) {
		r, e := ac.c.AdminControllerSetFacilitatorOfferAddressV1WithResponse(ctx, nil,
			threef.SetFacilitatorOfferAddressDto{OfferAddress: lowerAddr(addr)})
		if e != nil {
			return 0, e
		}
		resp = r
		return r.StatusCode(), nil
	})
	if err != nil {
		return errors.Errorf("3f api: set offer-address: %w", err)
	}
	if resp.JSON201 == nil {
		return errors.Errorf("3f api: set offer-address: unexpected status %s: %s", resp.Status(), string(resp.Body))
	}
	return nil
}

// lowerAddr renders an address as a lowercase hex string; the 3F API rejects checksummed addresses.
func lowerAddr(a common.Address) string { return strings.ToLower(a.Hex()) }
