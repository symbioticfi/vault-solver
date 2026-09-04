// Package app owns process lifecycle and the shared services injected into protocol integrations.
package app

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	"github.com/symbioticfi/vault-solver/internal/capacity"
	"github.com/symbioticfi/vault-solver/internal/chain"
	"github.com/symbioticfi/vault-solver/internal/observability"
	"github.com/symbioticfi/vault-solver/internal/signer"
	"github.com/symbioticfi/vault-solver/internal/txmanager"
)

// Services is the process-owned mechanism set injected into integration factories.
type Services struct {
	Chain     *chain.Client
	TxManager *txmanager.Manager
	Signer    signer.Signer
	Log       logr.Logger
	Metrics   *observability.Metrics
	Capacity  *capacity.Book
}

// Integration is one protocol workflow managed by the application.
type Integration interface {
	Name() string
	Run(context.Context) error
}

// ShutdownPreparer advertises the maximum time needed to retire external commitments after
// application cancellation starts.
type ShutdownPreparer interface {
	ShutdownPreparationTimeout() time.Duration
}
