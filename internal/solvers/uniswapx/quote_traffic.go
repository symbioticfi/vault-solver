package uniswapx

import "github.com/symbioticfi/vault-solver/internal/liquidlane"

type quoteHTTPObservation struct {
	request                   quoteRequest
	parsed, breaker, returned bool
	reason                    string
}

func (s *Solver) observeQuoteHTTP(observation *quoteHTTPObservation) {
	if s.metrics == nil || observation.breaker {
		return
	}
	var request liquidlane.QuoteMetricRequest
	if observation.parsed {
		r := observation.request
		request = liquidlane.QuoteMetricRequest{TokenIn: r.TokenIn, TokenOut: r.TokenOut, TokenInChainID: r.TokenInChainID, TokenOutChainID: r.TokenOutChainID, Amount: r.Amount}
		switch r.Type {
		case quoteTypeExactInput:
			request.Side = "input"
		case quoteTypeExactOutput:
			request.Side = "output"
		}
	}
	s.metrics.traffic.Observe(request, observation.returned, observation.reason)
}
