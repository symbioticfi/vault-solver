package uniswapx

import (
	"context"

	"github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
	"github.com/symbioticfi/vault-solver/internal/observability"
)

// decisionTrace returns the sink the strategy writes its planning rationale to. The lines carry the
// stage's trace ids, which is what joins a decision to the span that made it.
func (s *Solver) decisionTrace(ctx context.Context, baseFields ...any) strategies.DecisionTrace {
	log := observability.Log(ctx).V(1)
	if !log.Enabled() {
		return nil
	}
	base := append([]any(nil), baseFields...)
	return func(message string, keyValues ...any) {
		fields := make([]any, 0, len(base)+len(keyValues))
		fields = append(fields, base...)
		fields = append(fields, keyValues...)
		log.Info(message, fields...)
	}
}
