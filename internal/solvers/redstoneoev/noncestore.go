package redstoneoev

import "sync"

// nonceStore issues strictly-ascending EXECUTOR_V6 nonces. The Executor requires nonce >
// nonces[signer] and only advances that on a settled (or failed) execution, so we track the last
// issued nonce in memory and reconcile it with the on-chain value (which can jump ahead after a
// settlement we didn't initiate the next bid for). See docs/OEV-PLAN.md §6.2.
type nonceStore struct {
	mu     sync.Mutex
	issued uint64 // highest nonce handed out so far
}

// reconcile raises the in-memory high-water mark to the on-chain nonce (called at boot and from the
// ops loop). Never lowers it.
func (n *nonceStore) reconcile(onchain uint64) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reconcileLocked(onchain)
}

// next returns the next nonce to sign: strictly greater than both the on-chain nonce and any nonce
// already issued this session.
func (n *nonceStore) next(onchain uint64) uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.reconcileLocked(onchain)
	n.issued++
	return n.issued
}

func (n *nonceStore) reconcileLocked(onchain uint64) {
	if onchain > n.issued {
		n.issued = onchain
	}
}
