package uniswapx

import (
	"net/http"

	"github.com/go-errors/errors"

	"github.com/symbioticfi/vault-solver/internal/observability"
)

func recoverQuoteServer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		//nolint:contextcheck // recovery closure: it reads the request context to log, never passes one on.
		defer func() {
			if recovered := recover(); recovered != nil {
				observability.Log(request.Context()).Error(errors.Errorf("panic: %v", recovered),
					"quote server panic", "path", request.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, request)
	})
}
