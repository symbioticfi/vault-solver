package uniswapx

import (
	"net/http"

	"github.com/go-errors/errors"
	"github.com/go-logr/logr"
)

func recoverQuoteServer(next http.Handler, log logr.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Error(errors.Errorf("panic: %v", recovered), "quote server panic", "path", request.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, request)
	})
}
