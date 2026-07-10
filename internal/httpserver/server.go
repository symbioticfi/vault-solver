// Package httpserver owns joinable HTTP server lifecycles.
package httpserver

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/go-errors/errors"
)

// ServeUntil binds srv synchronously and serves until ctx is cancelled or the listener fails.
func ServeUntil(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.WithoutCancel(ctx), "tcp", srv.Addr)
	if err != nil {
		return errors.Errorf("listen %q: %w", srv.Addr, err)
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(listener)
	}()

	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return errors.Errorf("serve %q: %w", srv.Addr, err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	shutdownErr := srv.Shutdown(shutdownCtx)
	cancel()
	if shutdownErr != nil {
		// A timed-out graceful shutdown can leave Serve running. Force it closed before joining.
		_ = srv.Close()
	}

	err = <-serveErr
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.Errorf("serve %q: %w", srv.Addr, err)
	}
	if shutdownErr != nil {
		return errors.Errorf("shutdown %q: %w", srv.Addr, shutdownErr)
	}
	return nil
}
