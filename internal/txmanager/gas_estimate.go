package txmanager

import (
	"context"
	"math/big"
	"sync/atomic"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// Gas estimation modes (strategy §2.2). The legacy policy estimates at latest. The horizon policy
// estimates in the next block's context, eth_estimateGas with block overrides {N+1, t + blockTime} on top
// of the snapshot's header, which sees the interest accrual and state the fill will actually run against;
// when the read endpoint rejects the overrides (per call) or the capability probe found that it ignores
// them, it estimates at latest with the larger gas.fallbackHeadroomBps. Every estimate has its own
// gas.estimateTimeoutMs budget, separate from the fee reads, and is counted in gas_estimates_total{mode}.
//
// Goroutine model: estimates run on the worker's estimate goroutine; the probe loop (Start) is the only
// writer of Manager.overrides, an atomic the estimates read.

// Values of the sent log's estimateMode and of gas_estimates_total{mode}.
const (
	estimateModeSupplied  = "supplied"   // the request carried its gas limit; nothing was estimated
	estimateModeLatest    = "latest"     // plain estimate at latest: the legacy policy, or next-block estimates off
	estimateModeNextBlock = "next_block" // estimate with next-block overrides
	estimateModeFallback  = "fallback"   // plain estimate at latest because next-block estimates are unavailable
)

// Values of gas_estimates_total{outcome}.
const (
	gasEstimateOK          = "ok"
	gasEstimateReverted    = "revert"
	gasEstimateUnsupported = "unsupported"
	gasEstimateError       = "error"
)

// blockOverridesProbeInterval is how often the capability probe re-checks that the read endpoint honours
// block overrides, and blockOverridesProbeRetry how soon an inconclusive probe is repeated.
const (
	blockOverridesProbeInterval = 10 * time.Minute
	blockOverridesProbeRetry    = time.Minute
)

// nextBlockEstimateBackend estimates gas in the next block's context and probes whether the read endpoint
// honours that. *chain.Client provides it; without it the horizon policy estimates at latest.
type nextBlockEstimateBackend interface {
	EstimateGasWithBlockOverrides(
		ctx context.Context, msg ethereum.CallMsg, parent *big.Int, overrides ethereum.BlockOverrides,
	) (uint64, error)
	ProbeBlockOverrides(ctx context.Context, blockTime time.Duration) (bool, error)
}

// blockOverridesSupport is the capability probe's latest verdict.
type blockOverridesSupport int32

const (
	blockOverridesUnknown blockOverridesSupport = iota
	blockOverridesSupported
	blockOverridesUnsupported
)

// overridesState holds the probe verdict; its zero value is unknown.
type overridesState struct {
	value atomic.Int32
}

func (s *overridesState) load() blockOverridesSupport {
	return blockOverridesSupport(s.value.Load())
}

// swap stores next and returns the previous verdict.
func (s *overridesState) swap(next blockOverridesSupport) blockOverridesSupport {
	return blockOverridesSupport(s.value.Swap(int32(next)))
}

// gasEstimator runs one estimate for a request and reports its mode.
type gasEstimator func(ctx context.Context) (gas uint64, mode string, err error)

// callMsg is the call a request's gas is estimated for.
func (m *Manager) callMsg(req Request) ethereum.CallMsg {
	return ethereum.CallMsg{
		From:  m.signer.Address(),
		To:    &req.To,
		Value: req.Value,
		Data:  req.Data,
	}
}

// estimateHorizonGas estimates a horizon-policy request's gas limit on top of head, the header its fees
// are priced from. Only a next-block estimate the probe has confirmed is used with gas.headroomBps: until
// the first conclusive probe, and after one that found the overrides ignored, the plain estimate with
// gas.fallbackHeadroomBps is used, since an upstream that ignores them answers with the parent block's
// estimate. A per-call rejection of the overrides falls back the same way; a revert or any other error
// fails the send (nothing is signed and the nonce is not consumed).
func (m *Manager) estimateHorizonGas(ctx context.Context, req Request, head *types.Header) (uint64, string, error) {
	msg := m.callMsg(req)
	if m.nextBlock != nil && m.overrides.load() == blockOverridesSupported {
		overrides := chain.NextBlockOverrides(head, m.cfg.Fees.BlockTime)
		gas, err := m.boundedEstimate(ctx, req.Label, estimateModeNextBlock, func(ctx context.Context) (uint64, error) {
			return m.nextBlock.EstimateGasWithBlockOverrides(ctx, msg, head.Number, overrides)
		})
		if err == nil {
			return m.gasLimit(req, gas, m.gasHeadroomBps(), estimateModeNextBlock)
		}
		if !chain.IsBlockOverridesUnsupported(err) {
			return 0, estimateModeNextBlock, m.gasEstimateFailed(ctx, req, estimateModeNextBlock, err)
		}
		observability.Log(ctx).Info("next-block gas estimate rejected by the read endpoint; estimating at latest",
			"label", req.Label, "error", err.Error())
	}
	mode, headroom := estimateModeFallback, m.fallbackGasHeadroomBps()
	if m.cfg.Gas.NextBlockEstimateDisabled {
		mode, headroom = estimateModeLatest, m.gasHeadroomBps()
	}
	gas, err := m.boundedEstimate(ctx, req.Label, mode, func(ctx context.Context) (uint64, error) {
		return m.backend.EstimateGas(ctx, msg)
	})
	if err != nil {
		return 0, mode, m.gasEstimateFailed(ctx, req, mode, err)
	}
	return m.gasLimit(req, gas, headroom, mode)
}

// boundedEstimate runs one estimate within gas.estimateTimeoutMs and records it.
func (m *Manager) boundedEstimate(
	ctx context.Context, label, mode string, estimate func(context.Context) (uint64, error),
) (uint64, error) {
	estimateCtx, cancel := context.WithTimeout(ctx, m.gasEstimateTimeout())
	defer cancel()
	started := time.Now()
	gas, err := estimate(estimateCtx)
	m.observeGasEstimate(ctx, label, mode, started, err)
	return gas, err
}

// gasLimit adds headroom basis points to an estimate.
func (m *Manager) gasLimit(req Request, estimate, headroomBps uint64, mode string) (uint64, string, error) {
	limit, err := gasLimitWithHeadroom(estimate, headroomBps)
	if err != nil {
		return 0, mode, errors.Errorf("estimate gas %q: %w", req.Label, err)
	}
	return limit, mode, nil
}

// gasEstimateFailed logs a failed estimate, unless the send abandoned it, and wraps its error.
func (m *Manager) gasEstimateFailed(ctx context.Context, req Request, mode string, err error) error {
	if !errors.Is(context.Cause(ctx), errEstimateAbandoned) {
		// Calldata can contain unpublished authorizations. Keep it out of error logs and Sentry.
		observability.Log(ctx).Error(err, "gas estimation failed", "label", req.Label, "estimateMode", mode)
	}
	return errors.Errorf("estimate gas %q: %w", req.Label, err)
}

// observeGasEstimate counts one estimate by mode and outcome and records its duration. An estimate the
// send abandoned because it failed first is not an estimate outcome and is not counted.
func (m *Manager) observeGasEstimate(ctx context.Context, label, mode string, started time.Time, err error) {
	if errors.Is(context.Cause(ctx), errEstimateAbandoned) {
		return
	}
	m.metrics.observeGasEstimate(label, mode, gasEstimateOutcome(err), time.Since(started))
}

func gasEstimateOutcome(err error) string {
	switch {
	case err == nil:
		return gasEstimateOK
	case chain.IsExecutionReverted(err):
		return gasEstimateReverted
	case chain.IsBlockOverridesUnsupported(err):
		return gasEstimateUnsupported
	default:
		return gasEstimateError
	}
}

func (m *Manager) gasHeadroomBps() uint64 {
	if m.cfg.Gas.HeadroomBps == nil {
		return defaultGasHeadroomBps
	}
	return *m.cfg.Gas.HeadroomBps
}

func (m *Manager) fallbackGasHeadroomBps() uint64 {
	if m.cfg.Gas.FallbackHeadroomBps == nil {
		return defaultFallbackGasHeadroomBps
	}
	return *m.cfg.Gas.FallbackHeadroomBps
}

func (m *Manager) gasEstimateTimeout() time.Duration {
	return orDefault(m.cfg.Gas.EstimateTimeout, defaultGasEstimateTimeout)
}

// probeBlockOverrides runs the capability probe at startup and then every blockOverridesProbeInterval
// (blockOverridesProbeRetry after an inconclusive one) until ctx ends. It runs only under the horizon
// policy with next-block estimates on and a backend that can estimate with overrides.
func (m *Manager) probeBlockOverrides(ctx context.Context) {
	if m.nextBlock == nil {
		return
	}
	for {
		if sleepContext(ctx, m.probeBlockOverridesOnce(ctx)) != nil {
			return
		}
	}
}

// probeBlockOverridesOnce runs one probe as its own trace, records its verdict, and returns the delay until
// the next one. An inconclusive probe keeps the previous verdict; its first failure in a run and the run's
// end are logged at Info, and a run lasting readFailureReminderInterval at Error.
func (m *Manager) probeBlockOverridesOnce(ctx context.Context) time.Duration {
	probeCtx, end := tracer.Start(ctx, "txmanager.block_overrides_probe")
	timeoutCtx, cancel := context.WithTimeout(probeCtx, m.gasEstimateTimeout())
	supported, err := m.nextBlock.ProbeBlockOverrides(timeoutCtx, m.cfg.Fees.BlockTime)
	cancel()
	end(err)
	if ctx.Err() != nil {
		return 0
	}
	if err != nil {
		m.probeReads.failed(observability.Log(ctx), err,
			"block overrides probe inconclusive; keeping the previous gas estimate mode",
			"nextBlockEstimates", m.overrides.load() == blockOverridesSupported)
		return blockOverridesProbeRetry
	}
	m.probeReads.recovered(observability.Log(ctx), "block overrides probe recovered")
	verdict := blockOverridesUnsupported
	if supported {
		verdict = blockOverridesSupported
	}
	if previous := m.overrides.swap(verdict); previous != verdict {
		if supported {
			observability.Log(ctx).Info("read endpoint honours eth_estimateGas block overrides; estimating gas in " +
				"the next block's context")
		} else {
			observability.Log(ctx).Info("read endpoint rejects or ignores eth_estimateGas block overrides; estimating "+
				"gas at latest with gas.fallbackHeadroomBps", "fallbackHeadroomBps", m.fallbackGasHeadroomBps())
		}
	}
	return blockOverridesProbeInterval
}
