package strategies

import (
	"sort"
	"sync"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

type Factory func(raw yaml.Node) (types.Strategy, error)

var (
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("uniswapx strategy: Register called with empty name")
	}
	if f == nil {
		panic("uniswapx strategy: Register called with nil factory for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("uniswapx strategy: duplicate registration for " + name)
	}
	registry[name] = f
}

func New(name string, raw yaml.Node) (types.Strategy, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, errors.Errorf("unknown UniswapX strategy %q (registered: %v)", name, Registered())
	}
	return f(raw)
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
