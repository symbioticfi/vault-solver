// Package bridgefacilitator implements the 3F Bridge Facilitator integration.
package bridgefacilitator

import (
	"context"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/app"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

const Name = "3f-bridge-facilitator"

var inactiveOfferStatus = map[string]struct{}{
	"FAILED": {}, "NOT_ACCEPTED": {}, "CANCELLED": {}, "CANCELED": {},
}

type offerSigner interface {
	SignHash(common.Hash) ([]byte, error)
}

type transactionSender interface {
	Send(ctx context.Context, request txmanager.Request) txmanager.Result
}

// Solver is a single-goroutine protocol coordinator. Mutable offer and target state never escapes Run.
type Solver struct {
	cfg     *Config
	api     *apiClient
	reader  *reader
	planner Planner
	signer  offerSigner
	txm     transactionSender
	log     logr.Logger
	metrics *threeFMetrics

	laneReady  func() bool
	signerAddr common.Address
	probe      signerProbe
	nonceSeq   uint64
	offers     *offerTracker
	targets    []Target
	redeemable atomic.Pointer[[]Target]
}

type adapterOffering struct {
	target Target
	st     exposureState
}

type offerPass struct {
	input             OfferInput
	auctionByID       map[int64]auctionView
	exposureByAdapter map[common.Address]exposureState
}

func ValidateConfig(raw yaml.Node) error {
	config, err := parseConfig(raw)
	if err != nil {
		return err
	}
	if err := validatePlannerConfig(config.Strategy); err != nil {
		return errors.Errorf("strategy: %w", err)
	}
	return nil
}

func Factory(raw yaml.Node, services app.Services) (app.Integration, error) {
	config, err := parseConfig(raw)
	if err != nil {
		return nil, err
	}
	planner, err := newPlanner(config.Strategy)
	if err != nil {
		return nil, err
	}
	probe, err := newSignerProbe(services.Signer)
	if err != nil {
		return nil, err
	}

	coordinator := &Solver{
		cfg:        config,
		api:        newAPIClient(config.APIBaseURL, services.Signer, services.Chain.ChainID(), config.HTTPTimeout),
		reader:     newReader(services.Chain, config.LiquidityLens),
		planner:    planner,
		signer:     services.Signer,
		txm:        services.TxManager,
		log:        services.Log.WithName(Name),
		laneReady:  services.TxManager.LaneReady,
		signerAddr: services.Signer.Address(),
		probe:      probe,
		nonceSeq:   uint64(time.Now().UnixNano()),
		offers:     newOfferTracker(),
	}
	if services.Metrics != nil {
		coordinator.metrics, err = newThreeFMetrics(services.Metrics.Registerer(), config.Strategy.Name)
		if err != nil {
			return nil, err
		}
	}
	return coordinator, nil
}

func (solver *Solver) Name() string { return Name }

func deduplicateAdapters(adapters []common.Address) []common.Address {
	seen := make(map[common.Address]struct{}, len(adapters))
	result := make([]common.Address, 0, len(adapters))
	for _, adapter := range adapters {
		if _, exists := seen[adapter]; exists {
			continue
		}
		seen[adapter] = struct{}{}
		result = append(result, adapter)
	}
	return result
}

func (solver *Solver) nextNonce() uint64 {
	solver.nonceSeq++
	return solver.nonceSeq
}

func activeOffer(status string) bool {
	_, inactive := inactiveOfferStatus[strings.ToUpper(strings.TrimSpace(status))]
	return !inactive
}

func parsePrincipal(raw string) (*big.Int, bool) {
	return new(big.Int).SetString(raw, 10)
}
