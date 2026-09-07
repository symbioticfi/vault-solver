package planning

import (
	"math/big"
	"time"

	"github.com/go-errors/errors"
	"github.com/symbioticfi/vault-solver/internal/parse"
)

// ExecutionConfig is the common quote/fill safety policy used by direct LiquidLane integrations.
type ExecutionConfig struct {
	PriceBufferBps          int    `yaml:"priceBufferBps"`
	MinAmount               string `yaml:"minAmount"`
	InventoryReserveBps     int    `yaml:"inventoryReserveBps"`
	ExecutionDeadlineBuffer string `yaml:"executionDeadlineBuffer"`
}

// ExecutionPolicy contains only validated runtime values. Raw config is discarded after construction.
type ExecutionPolicy struct {
	PriceBufferBps      int
	InventoryReserveBps int
	MinAmount           *big.Int
	ExecutionBuffer     time.Duration
}

func (cfg ExecutionConfig) Parse() (ExecutionPolicy, error) {
	for _, field := range []struct {
		name  string
		value int
	}{
		{"priceBufferBps", cfg.PriceBufferBps}, {"inventoryReserveBps", cfg.InventoryReserveBps},
	} {
		if field.value < 0 || field.value >= 10_000 {
			return ExecutionPolicy{}, errors.Errorf("%s: must be in [0,10000), got %d", field.name, field.value)
		}
	}
	if cfg.PriceBufferBps >= 5_000 {
		return ExecutionPolicy{}, errors.New("2 * priceBufferBps: must be < 10000")
	}
	minimum, err := parse.Big(parse.OrDefault(cfg.MinAmount, "1"), "minAmount")
	if err != nil {
		return ExecutionPolicy{}, err
	}
	if minimum.Sign() <= 0 {
		return ExecutionPolicy{}, errors.New("minAmount: must be positive")
	}
	buffer, err := parse.Duration(cfg.ExecutionDeadlineBuffer, 12*time.Second, "executionDeadlineBuffer")
	if err != nil {
		return ExecutionPolicy{}, err
	}
	return ExecutionPolicy{PriceBufferBps: cfg.PriceBufferBps, InventoryReserveBps: cfg.InventoryReserveBps,
		MinAmount: minimum, ExecutionBuffer: buffer}, nil
}
