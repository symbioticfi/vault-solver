package webhook

import (
	"slices"
	"strconv"

	"github.com/go-errors/errors"
)

type HTTPStatusError struct {
	statusCode   int
	responseBody string
}

func (e *HTTPStatusError) Error() string {
	return "webhook: status " + strconv.Itoa(e.statusCode) + ": " + e.responseBody
}

func (e *HTTPStatusError) StatusCode() int {
	return e.statusCode
}

func IsHTTPStatus(err error, statusCodes ...int) bool {
	var statusErr *HTTPStatusError
	return errors.As(err, &statusErr) && slices.Contains(statusCodes, statusErr.statusCode)
}
