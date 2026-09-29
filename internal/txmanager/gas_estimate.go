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
// of the snapshot's header, which sees the interest accrual and state the fill will actually run against.
// That estimate is pinned to N like the balance read, and is retried in the same way while the upstream it
// lands on has not imported N yet, then refused as a stale head. When the read endpoint rejects the
// overrides (per call) or the capability probe found that it ignores them, it estimates at latest with the
// larger gas.fallbackHeadroomBps; until the first conclusive probe it does the same, counted as unconfirmed
// rather than as a fallback. Every estimate has its own gas.estimateTimeoutMs budget, separate from the fee
// reads, and is counted in gas_estimates_total{mode, outcome}.
//
// Goroutine model: estimates run on the worker's estimate goroutine; the probe loop (Start) is the only
// writer of Manager.overrides, an atomic the estimates read.

// Values of the sent log's estimateMode and of gas_estimates_total{mode}.
const (
	estimateModeSupplied  = "supplied"   // the request carried its gas limit; nothing was estimated
	estimateModeLatest    = "latest"     // plain estimate at latest: the legacy policy, or next-block estimates off
	estimateModeNextBlock = "next_block" // estimate with next-block overrides
	// estimateModeFallback is a plain estimate at latest because the read endpoint cannot give a next-block
	// one: the probe found the overrides rejected or ignored, a call rejected them, or the backend lacks
	// the capability. It is the mode an upstream alert follows.
	estimateModeFallback = "fallback"
	// estimateModeUnconfirmed is a plain estimate at latest before any conclusive probe: at startup, or
	// while every probe so far was inconclusive. It says nothing about the upstreams.
	estimateModeUnconfirmed = "unconfirmed"
)

// Values of gas_estimates_total{outcome}.
const (
	gasEstimateOK          = "ok"
	gasEstimateReverted    = "revert"
	gasEstimateUnsupported = "unsupported"
	// gasEstimateBlockNotFound is a next-block estimate whose parent block the node still did not have when
	// its budget ended; the send was refused as a stale head.
	gasEstimateBlockNotFound = "block_not_found"
	gasEstimateError         = "error"
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
// are priced from (see horizonEstimate), and adds the mode's headroom. A parent block the node never served
// within the budget refuses the send with ErrStaleHead (NotAdmitted, logged by the broadcast at Info); a
// revert or any other error is logged here and fails it. Nothing is signed and the nonce is not consumed.
func (m *Manager) estimateHorizonGas(ctx context.Context, req Request, head *types.Header) (uint64, string, error) {
	estimate, err := m.horizonEstimate(ctx, req, head)
	switch {
	case errors.Is(err, ErrStaleHead):
		return 0, estimate.mode, errors.Errorf("estimate gas %q: %w", req.Label, err)
	case err != nil:
		return 0, estimate.mode, m.gasEstimateFailed(ctx, req, estimate.mode, err)
	}
	return m.gasLimit(req, estimate.gas, estimate.headroomBps, estimate.mode)
}

// horizonGasEstimate is one raw horizon-policy estimate, the headroom its mode adds, and the mode.
type horizonGasEstimate struct {
	gas, headroomBps uint64
	mode             string
}

// horizonEstimate runs one horizon-policy estimate of req on top of head and returns the raw estimate, the
// headroom its mode adds, and the mode (set on error too). Only a next-block estimate the probe has confirmed is used with
// gas.headroomBps: until the first conclusive probe (unconfirmed), and after one that found the overrides
// ignored (fallback), the plain estimate with gas.fallbackHeadroomBps is used, since an upstream that ignores
// them answers with the parent block's estimate. A per-call rejection of the overrides falls back the same
// way. A parent block the node never served within the budget is ErrStaleHead. It logs no failure: a new
// send fails on one, while a pending lifecycle's re-estimate (strategy §2.7) decides what a revert means.
func (m *Manager) horizonEstimate(ctx context.Context, req Request, head *types.Header) (horizonGasEstimate, error) {
	msg := m.callMsg(req)
	verdict := m.overrides.load()
	if m.nextBlock != nil && verdict == blockOverridesSupported {
		overrides := chain.NextBlockOverrides(head, m.cfg.Fees.BlockTime)
		gas, err := m.boundedEstimate(ctx, req.Label, estimateModeNextBlock, func(ctx context.Context) (uint64, error) {
			return m.nextBlockEstimate(ctx, msg, head.Number, overrides)
		})
		switch {
		case err == nil:
			return horizonGasEstimate{gas: gas, headroomBps: m.gasHeadroomBps(), mode: estimateModeNextBlock}, nil
		case !chain.IsBlockOverridesUnsupported(err):
			return horizonGasEstimate{mode: estimateModeNextBlock}, err
		}
		observability.Log(ctx).Info("next-block gas estimate rejected by the read endpoint; estimating at latest",
			"label", req.Label, "error", err.Error())
	}
	mode, headroom := estimateModeFallback, m.fallbackGasHeadroomBps()
	switch {
	case m.cfg.Gas.NextBlockEstimateDisabled:
		mode, headroom = estimateModeLatest, m.gasHeadroomBps()
	case m.nextBlock != nil && verdict == blockOverridesUnknown:
		mode = estimateModeUnconfirmed
	}
	gas, err := m.boundedEstimate(ctx, req.Label, mode, func(ctx context.Context) (uint64, error) {
		return m.backend.EstimateGas(ctx, msg)
	})
	if err != nil {
		return horizonGasEstimate{mode: mode}, err
	}
	return horizonGasEstimate{gas: gas, headroomBps: headroom, mode: mode}, nil
}

// nextBlockEstimate runs the next-block estimate on top of parent until ctx, the estimate's budget, ends.
// parent is the snapshot's header, which one upstream reported; another that has not imported it yet
// answers "header not found", and eRPC does not fail over on that. Like the pinned balance read (strategy
// §2.4) the estimate is retried then, and a parent still missing when the budget ends is ErrStaleHead: it
// never falls back to latest, which on that upstream is an older block than the one the fees are priced at.
func (m *Manager) nextBlockEstimate(
	ctx context.Context, msg ethereum.CallMsg, parent *big.Int, overrides ethereum.BlockOverrides,
) (uint64, error) {
	retry := minPositiveDuration(pinnedReadRetryDelay, m.cfg.PollInterval)
	var notFound error
	for {
		gas, err := m.nextBlock.EstimateGasWithBlockOverrides(ctx, msg, parent, overrides)
		switch {
		case err == nil:
			return gas, nil
		case chain.IsBlockNotFound(err):
			notFound = err
		case notFound == nil || ctx.Err() == nil:
			return 0, err
		}
		// The node lacked parent, on this call or on the one before a retry the budget cut short.
		if sleepContext(ctx, retry) != nil {
			return 0, errors.Errorf("%w: next-block gas estimate parent block %s not found within %s: %w",
				ErrStaleHead, parent, m.gasEstimateTimeout(), notFound)
		}
	}
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
	case chain.IsBlockNotFound(err):
		return gasEstimateBlockNotFound
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
