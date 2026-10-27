// Package merklehost supplies shared host configuration for Merkle SDK workflows.
package merklehost

import (
	"net/http"
	"os"
	"time"

	"github.com/symbioticfi/merkle-executor/sdk/permissions"
	"github.com/symbioticfi/merkle-executor/sdk/protocolio"
	"github.com/symbioticfi/merkle-executor/sdk/provider"
)

// Curator contains the operator-supplied permission service settings.
type Curator struct {
	URL       string `yaml:"url" json:"url"`
	APIKeyEnv string `yaml:"apiKeyEnv,omitempty" json:"apiKeyEnv,omitempty"`
}

func (c Curator) HTTPConfig() protocolio.HTTPConfig {
	return protocolio.HTTPConfig{URL: c.URL, APIKeyEnv: c.APIKeyEnv, TimeoutSeconds: permissions.HostTimeoutSeconds, MaxResponseBytes: permissions.HostMaxResponseBytes}
}

func (c Curator) PermissionSource(chain provider.Reader, timeout time.Duration) (*permissions.Source, error) {
	return permissions.New(c.HTTPConfig(), &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, chain, os.Getenv)
}
