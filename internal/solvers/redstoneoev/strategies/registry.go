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
	Chain    *chain.Client
	Signer   signer.Signer
	Log      logr.Logger
	ChainID  int64
	Adapter  common.Address
	Callback common.Address
}

type Factory func(raw yaml.Node, deps Deps) (types.Strategy, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("OEV strategy: Register called with empty name")
	}
	if f == nil {
		panic("OEV strategy: Register called with nil factory for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("OEV strategy: duplicate registration for " + name)
	}
	registry[name] = f
}

func New(name string, raw yaml.Node, deps Deps) (types.Strategy, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, errors.Errorf("unknown OEV strategy %q (registered: %v)", name, Registered())
	}
	return f(raw, deps)
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
