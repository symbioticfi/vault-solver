package liquidlane

import (
	"fmt"
	"math/big"
)

// OracleBounds limits a signed discount to one range of undiscounted payout values.
// Min is inclusive, Max is exclusive. Both use the payout token's base units.
type OracleBounds struct {
	Min, Max *big.Int
}

func ParseOracleBounds(minimum, maximum string) (OracleBounds, error) {
	var bounds OracleBounds
	for _, field := range []struct {
		raw string
		to  **big.Int
	}{{minimum, &bounds.Min}, {maximum, &bounds.Max}} {
		if field.raw == "" {
			continue
		}
		value, ok := new(big.Int).SetString(field.raw, 10)
		if !ok || value.Sign() <= 0 || value.BitLen() > 128 {
			return OracleBounds{}, fmt.Errorf("invalid connector oracle cutoff %q", field.raw)
		}
		*field.to = value
	}
	if bounds.Min != nil && bounds.Max != nil && bounds.Min.Cmp(bounds.Max) >= 0 {
		return OracleBounds{}, fmt.Errorf("invalid connector oracle range")
	}
	return bounds, nil
}

func (b OracleBounds) Contains(gross *big.Int) bool {
	return (b.Min == nil || gross.Cmp(b.Min) >= 0) && (b.Max == nil || gross.Cmp(b.Max) < 0)
}
