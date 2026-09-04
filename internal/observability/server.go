package observability

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewHTTPServer(address string, metrics *Metrics) *http.Server {
	router := http.NewServeMux()
	router.Handle("/metrics", promhttp.HandlerFor(metrics.registry, promhttp.HandlerOpts{}))
	router.HandleFunc("/healthz", func(response http.ResponseWriter, _ *http.Request) {
		writeText(response, http.StatusOK, "ok")
	})
	router.HandleFunc("/readyz", func(response http.ResponseWriter, _ *http.Request) {
		if metrics.Ready() {
			writeText(response, http.StatusOK, "ready")
			return
		}
		writeText(response, http.StatusServiceUnavailable, "not ready")
	})
	return &http.Server{Addr: address, Handler: router, ReadHeaderTimeout: 5 * time.Second}
}

func writeText(response http.ResponseWriter, status int, body string) {
	response.WriteHeader(status)
	_, _ = response.Write([]byte(body))
}

// ServeUntil owns the server goroutine and joins graceful shutdown before returning.
func ServeUntil(ctx context.Context, server *http.Server, log logr.Logger) {
	serveResult := make(chan error, 1)
	go func() {
		err := server.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveResult <- err
			return
		}
		serveResult <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-serveResult:
		if err != nil {
			log.Error(err, "observability server failed")
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}
