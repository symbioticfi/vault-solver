package strategies

import (
	"sort"
	"sync"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"

	"github.com/symbioticfi/vault-solver/internal/solvers/uniswapx/strategies/types"
)

type Factory func(raw yaml.Node) (types.Strategy, error)
type Validator func(raw yaml.Node) error

type Registration struct {
	Factory        Factory
	ValidateConfig Validator
}

var (
	mu       sync.RWMutex
	registry = map[string]Registration{}
)

func Register(name string, registration Registration) {
	mu.Lock()
	defer mu.Unlock()
	if name == "" {
		panic("uniswapx strategy: Register called with empty name")
	}
	if registration.Factory == nil {
		panic("uniswapx strategy: Register called with nil factory for " + name)
	}
	if registration.ValidateConfig == nil {
		panic("uniswapx strategy: Register called with nil config validator for " + name)
	}
	if _, dup := registry[name]; dup {
		panic("uniswapx strategy: duplicate registration for " + name)
	}
	registry[name] = registration
}

func New(name string, raw yaml.Node) (types.Strategy, error) {
	registration, ok := lookup(name)
	if !ok {
		return nil, errors.Errorf("unknown UniswapX strategy %q (registered: %v)", name, Registered())
	}
	return registration.Factory(raw)
}

func Validate(name string, raw yaml.Node) error {
	registration, ok := lookup(name)
	if !ok {
		return errors.Errorf("unknown UniswapX strategy %q (registered: %v)", name, Registered())
	}
	return registration.ValidateConfig(raw)
}

func lookup(name string) (Registration, bool) {
	mu.RLock()
	defer mu.RUnlock()
	registration, ok := registry[name]
	return registration, ok
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
