// Package tokenpolicy owns immutable input-token admission and single-route policy.
package tokenpolicy

import (
	"slices"

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

// Policy's zero value admits every token. The sorted membership list is owned
// by the policy; callers receive separate maps when a strategy needs a set.
type Policy struct {
	scope  Scope
	tokens []common.Address
}

func Parse(scope string, addresses []string) (Policy, error) {
	tokens, err := parse.Addresses(addresses, "permissionedTokens")
	if err != nil {
		return Policy{}, err
	}
	policy, err := New(Scope(scope), tokens)
	if err != nil {
		return Policy{}, errors.Errorf("tokensToQuote: %w", err)
	}
	return policy, nil
}

func New(scope Scope, tokens []common.Address) (Policy, error) {
	switch scope {
	case "":
		scope = All
	case All, Permissioned, Permissionless:
	default:
		return Policy{}, errors.Errorf("invalid token scope %q", scope)
	}
	owned := slices.Clone(tokens)
	slices.SortFunc(owned, common.Address.Cmp)
	for index, address := range owned {
		if address == (common.Address{}) {
			return Policy{}, errors.New("permissionedTokens: zero address")
		}
		if index > 0 && address == owned[index-1] {
			return Policy{}, errors.Errorf("permissionedTokens: duplicate token %s", address)
		}
	}
	return Policy{scope: scope, tokens: owned}, nil
}

func (p Policy) Scope() Scope { return parse.OrDefault(p.scope, All) }

func (p Policy) contains(token common.Address) bool {
	_, found := slices.BinarySearchFunc(p.tokens, token, common.Address.Cmp)
	return found
}

func (p Policy) Allows(token common.Address) bool {
	scope := p.Scope()
	return scope == All || scope == Permissioned && p.contains(token) || scope == Permissionless && !p.contains(token)
}

func (p Policy) RequiresSingleRoute(token common.Address) bool {
	return p.Scope() == Permissioned && p.contains(token)
}

func (p Policy) SingleRouteTokens() map[common.Address]bool {
	if p.Scope() != Permissioned || len(p.tokens) == 0 {
		return nil
	}
	members := make(map[common.Address]bool, len(p.tokens))
	for _, token := range p.tokens {
		members[token] = true
	}
	return members
}
