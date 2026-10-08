package txmanager

import "github.com/go-errors/errors"

var ErrLaneReserved = errors.New("signer lane reserved by durable work")

// Hold fences new admission for every other owner. Restore durable reservations
// during solver construction, before starting the shared sender and solvers.
func (m *Manager) Hold(owner string) error {
	if owner == "" {
		return errors.New("reservation owner required")
	}
	m.reservationMu.Lock()
	if m.reservation != "" && m.reservation != owner {
		m.reservationMu.Unlock()
		return ErrLaneReserved
	}
	m.reservation = owner
	m.reservationMu.Unlock()
	m.notifyLaneStateChange()
	return nil
}

// Release follows durable business acknowledgement, never an uncertain result.
func (m *Manager) Release(owner string) error {
	m.reservationMu.Lock()
	if owner == "" || m.reservation != owner {
		m.reservationMu.Unlock()
		return ErrLaneReserved
	}
	m.reservation = ""
	m.reservationMu.Unlock()
	m.notifyLaneStateChange()
	return nil
}

// RestoreConfirmedNonce restores a durable journal's canonically confirmed nonce
// floor before admitting other solver work. It never selects or broadcasts a nonce.
func (m *Manager) RestoreConfirmedNonce(nonce uint64) { m.rememberConfirmedNonce(nonce) }
