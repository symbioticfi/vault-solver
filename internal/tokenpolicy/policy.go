// Package tokenpolicy defines the shared input-token admission policy used by solvers.
package tokenpolicy

import (
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/parse"
)

// Scope selects which input-token class a solver serves.
type Scope string

const (
	All            Scope = "all"
	Permissioned   Scope = "permissioned"
	Permissionless Scope = "permissionless"
)

// Policy admits input tokens and marks the permissioned-only scope as single-route.
// Its zero value is the unrestricted "all" policy.
type Policy struct {
	scope        Scope
	permissioned map[common.Address]bool
}

// Parse validates the YAML-facing scope and permissioned-token list.
func Parse(rawScope string, rawTokens []string) (Policy, error) {
	scope := Scope(parse.OrDefault(rawScope, string(All)))
	if !validScope(scope) {
		return Policy{}, errors.Errorf(
			"tokensToQuote: must be %q, %q or %q, got %q",
			All, Permissioned, Permissionless, rawScope,
		)
	}

	tokens := make([]common.Address, 0, len(rawTokens))
	for i, value := range rawTokens {
		token, err := parse.NonZeroAddress(value, "permissionedTokens["+strconv.Itoa(i)+"]")
		if err != nil {
			return Policy{}, err
		}
		tokens = append(tokens, token)
	}
	return New(scope, tokens)
}

// New constructs a policy from typed values.
func New(scope Scope, tokens []common.Address) (Policy, error) {
	if scope == "" {
		scope = All
	}
	if !validScope(scope) {
		return Policy{}, errors.Errorf("invalid token scope %q", scope)
	}

	policy := Policy{scope: scope, permissioned: make(map[common.Address]bool, len(tokens))}
	for i, token := range tokens {
		if token == (common.Address{}) {
			return Policy{}, errors.Errorf("permissionedTokens[%d]: zero address", i)
		}
		if policy.permissioned[token] {
			return Policy{}, errors.Errorf("permissionedTokens[%d]: duplicate token %s", i, token.Hex())
		}
		policy.permissioned[token] = true
	}
	return policy, nil
}

func validScope(scope Scope) bool {
	return scope == All || scope == Permissioned || scope == Permissionless
}

// Scope returns the normalized configured scope.
func (p Policy) Scope() Scope {
	if p.scope == "" {
		return All
	}
	return p.scope
}

// Allows reports whether token is in this solver's input-token scope.
func (p Policy) Allows(token common.Address) bool {
	switch p.Scope() {
	case Permissioned:
		return p.permissioned[token]
	case Permissionless:
		return !p.permissioned[token]
	case All:
		return true
	}
	return false
}

// RequiresSingleRoute reports whether an admitted token must use one physical route.
func (p Policy) RequiresSingleRoute(token common.Address) bool {
	return p.Scope() == Permissioned && p.permissioned[token]
}

// SingleRouteTokens returns the strategy-facing single-route token set.
func (p Policy) SingleRouteTokens() map[common.Address]bool {
	if p.Scope() != Permissioned || len(p.permissioned) == 0 {
		return nil
	}
	tokens := make(map[common.Address]bool, len(p.permissioned))
	for token := range p.permissioned {
		tokens[token] = true
	}
	return tokens
}
