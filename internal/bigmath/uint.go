package bigmath

import "math/bits"

// SaturatingAdd and SaturatingMul clamp overflow to the largest uint64. Gas
// ceilings must remain conservative when untrusted counts exceed that range.
func SaturatingAdd(a, b uint64) uint64 {
	sum, carry := bits.Add64(a, b, 0)
	if carry != 0 {
		return ^uint64(0)
	}
	return sum
}

func SaturatingMul(a, b uint64) uint64 {
	high, low := bits.Mul64(a, b)
	if high != 0 {
		return ^uint64(0)
	}
	return low
}
