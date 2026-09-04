package parse

import (
	"bytes"

	"github.com/go-errors/errors"
	"gopkg.in/yaml.v3"
)

func DecodeStrict(node yaml.Node, destination any) error {
	if node.Kind == 0 {
		node = yaml.Node{Kind: yaml.MappingNode}
	}
	encoded, err := yaml.Marshal(&node)
	if err != nil {
		return errors.Errorf("re-encode config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(encoded))
	decoder.KnownFields(true)
	if err := decoder.Decode(destination); err != nil {
		return errors.Errorf("decode config: %w", err)
	}
	return nil
}
