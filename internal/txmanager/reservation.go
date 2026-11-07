package txmanager

import "github.com/go-errors/errors"

var ErrLaneReserved = errors.New("signer lane reserved by host-owned work")

// Hold fences new admission for every other owner until the host resolves its
// signed work. The reservation is process-local and has no restart recovery.
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

// Release follows canonical resolution or a definite pre-sign failure, never an
// uncertain signed result.
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
