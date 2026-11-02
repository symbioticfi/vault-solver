package uniswapx

import "github.com/ethereum/go-ethereum/common"

type quoteHTTPObservation struct {
	breaker, returned bool
	reason            string
	pairKnown         bool
	tokenIn, tokenOut common.Address
}

func (s *Solver) observeQuoteHTTP(observation *quoteHTTPObservation) {
	if s.metrics == nil || observation.breaker {
		return
	}
	s.metrics.traffic.Observe(observation.returned, observation.reason)
	if observation.pairKnown {
		s.metrics.pairs.Quote(s.chainID, observation.tokenIn, observation.tokenOut, observation.returned, observation.reason)
	}
}
