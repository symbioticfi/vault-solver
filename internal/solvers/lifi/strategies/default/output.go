package defaultstrategy

import (
	"encoding/binary"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
)

const (
	limitOrderContextType            = 0x00
	dutchAuctionContextType          = 0x01
	exclusiveLimitOrderContextType   = 0xe0
	exclusiveDutchAuctionContextType = 0xe1
)

type outputPricing struct {
	amount *big.Int

	startTime uint32

	exclusive    bool
	exclusiveFor [32]byte
}

func parseOutputContext(outputAmount *big.Int, encoded []byte) (*outputPricing, error) {
	if outputAmount == nil || outputAmount.Sign() <= 0 {
		return nil, errors.New("outputContext: output amount must be positive")
	}
	pricing := &outputPricing{amount: new(big.Int).Set(outputAmount)}
	if len(encoded) == 0 {
		return pricing, nil
	}
	var size int
	switch encoded[0] {
	case limitOrderContextType:
		size = 1
	case exclusiveLimitOrderContextType:
		size = 37
	case dutchAuctionContextType, exclusiveDutchAuctionContextType:
		return nil, errors.New("outputContext: Dutch auctions are not supported")
	default:
		return nil, errors.Errorf("outputContext: unsupported type 0x%02x", encoded[0])
	}
	if len(encoded) != size {
		label := "limit order"
		if size == 37 {
			label = "exclusive limit"
		}
		return nil, errors.Errorf("outputContext: %s length must be %d, got %d", label, size, len(encoded))
	}
	if size == 37 {
		pricing.exclusive = true
		pricing.exclusiveFor = [32]byte(encoded[1:33])
		pricing.startTime = binary.BigEndian.Uint32(encoded[33:])
	}
	return pricing, nil
}

func (o *outputPricing) fill(solver common.Address, now time.Time, acceptableAmount *big.Int) (*big.Int, bool) {
	if acceptableAmount == nil || o.amount.Cmp(acceptableAmount) > 0 {
		return nil, false
	}
	allowed := !o.exclusive || uint32Time(now) >= o.startTime || o.exclusiveFor == solverIdentifier(solver)
	if !allowed {
		return nil, false
	}
	return new(big.Int).Set(o.amount), true
}

func uint32Time(t time.Time) uint32 { return uint32(min(max(t.Unix(), 0), int64(math.MaxUint32))) }

func solverIdentifier(addr common.Address) [32]byte {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return out
}
