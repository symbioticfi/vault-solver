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

func parseOutputContext(outputAmount *big.Int, outputContext []byte) (*outputPricing, error) {
	out := &outputPricing{amount: new(big.Int).Set(outputAmount)}
	if len(outputContext) == 0 {
		return out, nil
	}
	switch outputContext[0] {
	case limitOrderContextType:
		if len(outputContext) != 1 {
			return nil, errors.Errorf("outputContext: limit order length must be 1, got %d", len(outputContext))
		}
		return out, nil
	case dutchAuctionContextType:
		return nil, errors.New("outputContext: Dutch auctions are not supported")
	case exclusiveLimitOrderContextType:
		if len(outputContext) != 37 {
			return nil, errors.Errorf("outputContext: exclusive limit length must be 37, got %d", len(outputContext))
		}
		out.exclusive = true
		copy(out.exclusiveFor[:], outputContext[1:33])
		out.startTime = binary.BigEndian.Uint32(outputContext[33:37])
		return out, nil
	case exclusiveDutchAuctionContextType:
		return nil, errors.New("outputContext: Dutch auctions are not supported")
	default:
		return nil, errors.Errorf("outputContext: unsupported type 0x%02x", outputContext[0])
	}
}

func (o *outputPricing) fill(solver common.Address, now time.Time, acceptableAmount *big.Int) (*big.Int, bool) {
	currentTime := uint32Time(now)
	if o.exclusive && currentTime < o.startTime {
		solverID := solverIdentifier(solver)
		if o.exclusiveFor != solverID {
			return nil, false
		}
	}
	if o.amount.Cmp(acceptableAmount) > 0 {
		return nil, false
	}
	return new(big.Int).Set(o.amount), true
}

func uint32Time(t time.Time) uint32 {
	unix := t.Unix()
	if unix <= 0 {
		return 0
	}
	if unix > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(unix)
}

func solverIdentifier(addr common.Address) [32]byte {
	var out [32]byte
	copy(out[12:], addr.Bytes())
	return out
}
