//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
)

type testEnvironment struct {
	profile     string
	variant     string
	artifactDir string
	fixtureURL  string
	backendURL  string
	rfqURL      string
	quoteURL    string
	metricsURL  string
	manifest    deploymentManifest
	client      *ethclient.Client
	httpClient  *http.Client
}

type deploymentManifest struct {
	Chain        manifestChain        `json:"chain"`
	Contracts    manifestContracts    `json:"contracts"`
	Participants manifestParticipants `json:"participants"`
	Tokens       manifestTokens       `json:"tokens"`
	Vaults       []manifestVault      `json:"vaults"`
	Lifi         lifiManifest         `json:"lifi"`
	UniswapX     uniswapManifest      `json:"uniswapx"`
	OEV          oevManifest          `json:"oev"`
	ThreeF       threeFManifest       `json:"threef"`
}

type manifestChain struct {
	ID         int64  `json:"id"`
	StartBlock uint64 `json:"startBlock"`
}

type manifestContracts struct {
	Reactor  common.Address `json:"reactor"`
	Executor common.Address `json:"executor"`
}

type manifestParticipants struct {
	MarketMaker common.Address `json:"marketMaker"`
}

type manifestTokens struct {
	Input         []manifestToken `json:"input"`
	Output        []manifestToken `json:"output"`
	DefaultInput  common.Address  `json:"defaultInput"`
	DefaultOutput common.Address  `json:"defaultOutput"`
}

type manifestToken struct {
	Address  common.Address `json:"address"`
	Decimals uint8          `json:"decimals"`
}

type manifestVault struct {
	Adapter common.Address `json:"adapter"`
	Asset   common.Address `json:"asset"`
}

type lifiManifest struct {
	InputSettler  common.Address   `json:"inputSettler"`
	OutputSettler common.Address   `json:"outputSettler"`
	Executor      common.Address   `json:"executor"`
	Adapters      []common.Address `json:"adapters"`
	Permissioned  []common.Address `json:"permissionedTokens"`
	TokenIn       common.Address   `json:"tokenIn"`
	TokenOut      common.Address   `json:"tokenOut"`
}

type uniswapManifest struct {
	Executor common.Address   `json:"executor"`
	Adapters []common.Address `json:"adapters"`
	TokenIn  common.Address   `json:"tokenIn"`
	TokenOut common.Address   `json:"tokenOut"`
}

type oevManifest struct {
	Executor  common.Address `json:"executor"`
	Callback  common.Address `json:"callback"`
	Adapter   common.Address `json:"adapter"`
	LoanToken common.Address `json:"loanToken"`
	Morpho    common.Address `json:"morpho"`
	Bid       oevBid         `json:"bid"`
	Sizing    oevSizing      `json:"sizing"`
	Markets   []oevMarket    `json:"markets"`
}

type oevBid struct {
	BidETH string `json:"bidEth"`
}

type oevSizing struct {
	SwapHaircutBPS int64 `json:"swapHaircutBps"`
}

type oevMarket struct {
	ID              common.Hash    `json:"id"`
	CollateralToken common.Address `json:"collateralToken"`
	Borrower        common.Address `json:"borrower"`
	AuctionPrice    string         `json:"auctionPrice"`
}

type threeFManifest struct {
	Adapter common.Address `json:"adapter"`
	Request common.Address `json:"request"`
	Asset   common.Address `json:"asset"`
	Auction threeFAuction  `json:"auction"`
}

type threeFAuction struct {
	MaxRateBPS float64 `json:"maxRateBps"`
}

func TestE2E(t *testing.T) {
	testEnv := loadTestEnvironment(t)
	t.Run(testEnv.profile+"/"+testEnv.variant, func(t *testing.T) {
		switch testEnv.profile {
		case "rfq":
			testRFQ(t, testEnv)
		case "lifi":
			testLifi(t, testEnv)
		case "uniswapx":
			testUniswapX(t, testEnv)
		case "3f":
			testThreeF(t, testEnv)
		case "redstoneoev":
			testOEV(t, testEnv)
		default:
			t.Fatalf("unsupported E2E profile %q", testEnv.profile)
		}
	})
}

func loadTestEnvironment(t *testing.T) *testEnvironment {
	t.Helper()
	required := func(name string) string {
		value := os.Getenv(name)
		if value == "" {
			t.Fatalf("%s is required", name)
		}
		return value
	}

	manifestPath := required("RFQ_DEPLOYMENT_MANIFEST")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read deployment manifest: %v", err)
	}
	var manifest deploymentManifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatalf("decode deployment manifest: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rpcURL := required("RFQ_RPC_URL")
	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		t.Fatalf("dial E2E RPC: %v", err)
	}
	t.Cleanup(client.Close)
	chainID, err := client.ChainID(ctx)
	if err != nil {
		t.Fatalf("read E2E chain ID: %v", err)
	}
	if chainID.Cmp(big.NewInt(manifest.Chain.ID)) != 0 {
		t.Fatalf("chain ID = %s, want %d", chainID, manifest.Chain.ID)
	}

	return &testEnvironment{
		profile:     required("VAULT_SOLVER_E2E_PROFILE"),
		variant:     required("VAULT_SOLVER_E2E_VARIANT"),
		artifactDir: required("VAULT_SOLVER_E2E_ARTIFACT_DIR"),
		fixtureURL:  required("LOCAL_FIXTURE_URL"),
		backendURL:  required("RFQ_LOCAL_BACKEND_URL"),
		rfqURL:      required("RFQ_SOLVER_URL"),
		quoteURL:    required("UNISWAPX_QUOTE_URL"),
		metricsURL:  required("VAULT_SOLVER_METRICS_URL"),
		manifest:    manifest,
		client:      client,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}
