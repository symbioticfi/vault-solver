package parse

import "gopkg.in/yaml.v3"

// NamedConfig selects a component while deferring its private configuration decoder.
type NamedConfig struct {
	Name   string    `yaml:"name"`
	Config yaml.Node `yaml:"config"`
}
