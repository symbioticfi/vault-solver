package httpclient

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/go-errors/errors"
)

type upstreamError struct{}

func (*upstreamError) Error() string { return "request rejected" }
func (*upstreamError) Body() []byte  { return []byte(" detail ") }

func TestExecute(t *testing.T) {
	original := &upstreamError{}
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "success"},
		{name: "upstream", err: original, want: "fetch: 400 Bad Request: detail"},
		{name: "transport", err: io.ErrUnexpectedEOF, want: "fetch: 400 Bad Request: unexpected EOF"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedBody{Reader: strings.NewReader("consumed by generated client")}
			output, err := Execute("fetch", func() (string, *http.Response, error) {
				return "decoded", &http.Response{Status: "400 Bad Request", Body: body}, tc.err
			})
			if body.closed != 1 {
				t.Fatalf("response closed %d times", body.closed)
			}
			if (tc.err == nil && output != "decoded") || (tc.err != nil && output != "") {
				t.Fatalf("output=%q err=%v", output, err)
			}
			if !errors.Is(err, tc.err) {
				t.Fatalf("error=%v does not wrap %v", err, tc.err)
			}
			if tc.want != "" && err.Error() != tc.want {
				t.Fatalf("error=%v, want %s", err, tc.want)
			}
		})
	}
	if _, err := Execute("fetch", func() (string, *http.Response, error) { return "", nil, original }); !errors.Is(err, original) || !strings.Contains(err.Error(), "no response") {
		t.Fatalf("missing response error=%v", err)
	}
}

type trackedBody struct {
	io.Reader

	closed int
}

func (body *trackedBody) Close() error { body.closed++; return nil }
