package planning

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/symbioticfi/vault-solver/internal/liquidlane"
)

func BenchmarkSolveFill(b *testing.B) {
	in, out := common.Address{19: 1}, common.Address{19: 2}
	for _, count := range []int{8, 24} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			task := FillTask{TokenIn: in, TokenOut: out, AmountIn: big.NewInt(1_000_000), MaxRoutes: 8}
			for route := range count {
				for alternative := range 3 {
					var discount *common.Hash
					if alternative > 0 {
						hash := common.Hash{30: byte(route), 31: byte(alternative)}
						discount = &hash
					}
					task.Quotes = append(task.Quotes, testFillQuote(liquidlane.RouteID(strconv.Itoa(route)),
						liquidlane.CapacityID(strconv.Itoa(route/2)), in, out, 1_000_000,
						int64(1_000_000+alternative*100_000), 400_000, discount))
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				solution, err := SolveFill(task)
				if err != nil || solution == nil || len(solution.Finalize(solution.MaxAmountOut())) == 0 {
					b.Fatalf("no complete fill: %v", err)
				}
			}
		})
	}
}
