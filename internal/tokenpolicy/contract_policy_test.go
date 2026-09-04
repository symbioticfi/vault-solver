package tokenpolicy

import (
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

var (
	permissionedToken   = common.HexToAddress("0x1111111111111111111111111111111111111111")
	permissionlessToken = common.HexToAddress("0x2222222222222222222222222222222222222222")
)

func TestPolicyScopes(t *testing.T) {
	tests := []struct {
		name               string
		scope              Scope
		token              common.Address
		wantAllowed        bool
		wantSingleRoute    bool
		wantSingleRouteSet bool
	}{
		{"zero defaults to all", "", permissionedToken, true, false, false},
		{"all admits permissioned", All, permissionedToken, true, false, false},
		{"all admits permissionless", All, permissionlessToken, true, false, false},
		{"permissioned admits member", Permissioned, permissionedToken, true, true, true},
		{"permissioned rejects non-member", Permissioned, permissionlessToken, false, false, true},
		{"permissionless rejects member", Permissionless, permissionedToken, false, false, false},
		{"permissionless admits non-member", Permissionless, permissionlessToken, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := New(tt.scope, []common.Address{permissionedToken})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := policy.Allows(tt.token); got != tt.wantAllowed {
				t.Fatalf("Allows() = %v, want %v", got, tt.wantAllowed)
			}
			if got := policy.RequiresSingleRoute(tt.token); got != tt.wantSingleRoute {
				t.Fatalf("RequiresSingleRoute() = %v, want %v", got, tt.wantSingleRoute)
			}
			_, gotSingleRouteSet := policy.SingleRouteTokens()[permissionedToken]
			if gotSingleRouteSet != tt.wantSingleRouteSet {
				t.Fatalf("SingleRouteTokens() contains permissioned token = %v, want %v", gotSingleRouteSet, tt.wantSingleRouteSet)
			}
		})
	}
}

func TestParseValidatesConfig(t *testing.T) {
	address := permissionedToken.Hex()
	policy, err := Parse("", []string{address})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if policy.Scope() != All || !policy.Allows(permissionlessToken) {
		t.Fatalf("default policy = %q", policy.Scope())
	}

	tests := []struct {
		name   string
		scope  string
		tokens []string
		match  string
	}{
		{"invalid scope", "private", nil, "tokensToQuote"},
		{"invalid address", "all", []string{"bad"}, "invalid address"},
		{"zero address", "all", []string{common.Address{}.Hex()}, "zero address"},
		{"duplicate", "all", []string{address, address}, "duplicate address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.scope, tt.tokens)
			if err == nil || !strings.Contains(err.Error(), tt.match) {
				t.Fatalf("Parse() error = %v, want match %q", err, tt.match)
			}
		})
	}
}

func TestSingleRouteTokensReturnsCopy(t *testing.T) {
	policy, err := New(Permissioned, []common.Address{permissionedToken})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tokens := policy.SingleRouteTokens()
	delete(tokens, permissionedToken)
	if !policy.RequiresSingleRoute(permissionedToken) {
		t.Fatal("caller mutated policy through SingleRouteTokens")
	}
}
