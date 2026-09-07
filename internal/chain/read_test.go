package chain

import (
	"testing"

	"github.com/go-errors/errors"
)

func TestDecodeChecksCallBeforeDecoder(t *testing.T) {
	decodeErr := errors.New("invalid ABI")
	for _, test := range []struct {
		name      string
		success   bool
		decodeErr error
		wantCall  bool
		wantErr   bool
	}{
		{name: "reverted call with decodable bytes", wantErr: true},
		{name: "malformed successful return", success: true, decodeErr: decodeErr, wantCall: true, wantErr: true},
		{name: "valid return", success: true, wantCall: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			value, err := Decode(CallResult{Success: test.success, ReturnData: []byte{42}}, func(data []byte) (byte, error) {
				called = true
				return data[0], test.decodeErr
			})
			if called != test.wantCall || (err != nil) != test.wantErr {
				t.Fatalf("called=%v value=%d err=%v", called, value, err)
			}
			if test.decodeErr != nil && !errors.Is(err, test.decodeErr) {
				t.Fatalf("decoder error lost: %v", err)
			}
			if !test.wantErr && value != 42 {
				t.Fatalf("value=%d", value)
			}
		})
	}
}
