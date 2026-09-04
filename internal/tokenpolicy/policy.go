// Package tokenpolicy owns the protocol-neutral input token admission rule.
package tokenpolicy

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

type Scope string

const (
	All            Scope = "all"
	Permissioned   Scope = "permissioned"
	Permissionless Scope = "permissionless"
)

// Policy is immutable after construction. Its zero value is the unrestricted policy.
type Policy struct {
	scope  Scope
	tokens map[common.Address]struct{}
}

func Parse(rawScope string, rawTokens []string) (Policy, error) {
	scope := Scope(parse.OrDefault(rawScope, string(All)))
	tokens, err := parse.NonZeroAddresses(rawTokens, "permissionedTokens")
	if err != nil {
		return Policy{}, err
	}
	return New(scope, tokens)
}

func New(scope Scope, tokens []common.Address) (Policy, error) {
	if scope == "" {
		scope = All
	}
	if !valid(scope) {
		return Policy{}, errors.Errorf(
			"tokensToQuote: must be %q, %q or %q, got %q",
			All, Permissioned, Permissionless, scope,
		)
	}
	policy := Policy{scope: scope, tokens: make(map[common.Address]struct{}, len(tokens))}
	for index, token := range tokens {
		if token == (common.Address{}) {
			return Policy{}, errors.Errorf("permissionedTokens[%d]: zero address", index)
		}
		if _, duplicate := policy.tokens[token]; duplicate {
			return Policy{}, errors.Errorf("permissionedTokens[%d]: duplicate token %s", index, token.Hex())
		}
		policy.tokens[token] = struct{}{}
	}
	return policy, nil
}

func valid(scope Scope) bool {
	return scope == All || scope == Permissioned || scope == Permissionless
}

func (p Policy) Scope() Scope {
	if p.scope == "" {
		return All
	}
	return p.scope
}

func (p Policy) Allows(token common.Address) bool {
	_, listed := p.tokens[token]
	switch p.Scope() {
	case All:
		return true
	case Permissioned:
		return listed
	case Permissionless:
		return !listed
	default:
		return false
	}
}

func (p Policy) RequiresSingleRoute(token common.Address) bool {
	_, listed := p.tokens[token]
	return p.Scope() == Permissioned && listed
}

func (p Policy) SingleRouteTokens() map[common.Address]bool {
	if p.Scope() != Permissioned || len(p.tokens) == 0 {
		return nil
	}
	tokens := make(map[common.Address]bool, len(p.tokens))
	for token := range p.tokens {
		tokens[token] = true
	}
	return tokens
}
