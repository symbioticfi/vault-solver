// Package merklehost supplies shared host configuration for Merkle SDK workflows.
package merklehost

import (
	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
)

// Curator contains the operator-supplied permission service settings.
type Curator struct {
	URL string `yaml:"url" json:"url"`
}

func (c Curator) PermissionSource(chain provider.Reader) (*permissions.Source, error) {
	return permissions.New(c.URL, nil, chain)
}
