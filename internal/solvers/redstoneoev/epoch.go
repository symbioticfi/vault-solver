package redstoneoev

import (
	"time"
)

type readEpoch struct {
	Block uint64
	At    time.Time
}

func newReadEpoch(block uint64, at time.Time) readEpoch {
	return readEpoch{Block: block, At: at}
}
