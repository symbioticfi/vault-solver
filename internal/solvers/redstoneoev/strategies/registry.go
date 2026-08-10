package strategies

import (
	"sort"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/solvers/redstoneoev/strategies/types"
)

type Deps struct {
	Chain               *chain.Client
	Signer              signer.Signer
	Log                 logr.Logger
	ChainID             int64
	Adapter             common.Address
	Callback            common.Address
	LoadAdapterSnapshot func() (types.AdapterSnapshot, bool)
	GasAccounting       bool
}

type Factory func(raw yaml.Node, deps Deps) (types.Strategy, error)

type Registration struct {
	Factory        Factory
	RequiresBidCap bool
}

var (
	mu       sync.RWMutex
	registry = map[string]Registration{}
)

func Register(name string, registration Registration) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("OEV strategy: Register called with empty name")
	}
	if registration.Factory == nil {
		panic("OEV strategy: Register called with nil factory for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("OEV strategy: duplicate registration for " + name)
	}
	registry[name] = registration
}

func New(name string, raw yaml.Node, deps Deps) (types.Strategy, error) {
	mu.RLock()
	registration, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, errors.Errorf("unknown OEV strategy %q (registered: %v)", name, Registered())
	}
	return registration.Factory(raw, deps)
}

func RequiresBidCap(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name].RequiresBidCap
}

func Registered() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
