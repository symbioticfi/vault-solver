package scalars

import (
	"bytes"
	"encoding/json"
)

// BigIntString accepts Morpho's BigInt scalar as either a JSON string or number and stores decimal text.
type BigIntString string

func (b *BigIntString) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*b = ""
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		*b = BigIntString(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*b = BigIntString(n.String())
	return nil
}

func (b BigIntString) String() string {
	return string(b)
}
