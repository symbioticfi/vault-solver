package uniswapx

import (
	liquidstrategies "github.com/symbioticfi/vault-solver/internal/liquidlane/strategies"
)

func (s *Solver) decisionTrace(baseFields ...any) liquidstrategies.DecisionTrace {
	log := s.log.V(1)
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
