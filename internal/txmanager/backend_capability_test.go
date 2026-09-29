package txmanager

import "github.com/symbioticfi/vault-solver/internal/chain"

// The production backend must keep the capabilities the manager detects at run time: the balance guard
// turns itself off, with a single Info line, for a backend without a pinned balance read, and the horizon
// policy estimates at latest for one without next-block estimates, so drift in chain.Client's method set
// has to break the default build rather than silently disable either.
var (
	_ Backend                  = (*chain.Client)(nil)
	_ pinnedBalanceBackend     = (*chain.Client)(nil)
	_ nextBlockEstimateBackend = (*chain.Client)(nil)
)
