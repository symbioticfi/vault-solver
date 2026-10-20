package threef

import (
	"bytes"
	"encoding/json"
	"io"
	"math/big"
	"strconv"
	"strings"

	"github.com/go-errors/errors"
)

// Check raw identifiers before float32 decoding, and reject ambiguous duplicate JSON properties.
func checkJSON(body []byte) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.UseNumber()
	if _, err := checkValue(d, 0); err != nil {
		return errors.New("3f invalid or ambiguous JSON response")
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.New("3f trailing JSON response data")
	}
	return nil
}

func checkValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON nesting exceeds bound")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return token, nil
	}
	if delim == '[' {
		for d.More() {
			if _, err = checkValue(d, depth+1); err != nil {
				return nil, err
			}
		}
		_, err = d.Token()
		return nil, err
	}
	if delim != '{' {
		return nil, errors.New("invalid JSON container")
	}
	seen := make(map[string]bool)
	for d.More() {
		keyToken, keyErr := d.Token()
		key, ok := keyToken.(string)
		if keyErr != nil || !ok || seen[key] {
			return nil, errors.New("duplicate or invalid JSON property")
		}
		seen[key] = true
		value, valueErr := checkValue(d, depth+1)
		if valueErr != nil {
			return nil, valueErr
		}
		if key == "id" || key == "auctionId" || key == "offerId" {
			number, isNumber := value.(json.Number)
			if !isNumber || !exactIDNumber(number) {
				return nil, errors.New("invalid exact JSON identifier")
			}
		}
	}
	_, err = d.Token()
	return nil, err
}

func exactIDNumber(number json.Number) bool {
	value := string(number)
	// Bound decimal expansion before big.Rat handles an attacker-controlled exponent.
	if len(value) > 64 {
		return false
	}
	if i := strings.IndexAny(value, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(value[i+1:])
		if err != nil || exponent < -64 || exponent > 64 {
			return false
		}
	}
	rational, valid := new(big.Rat).SetString(value)
	return valid && rational.IsInt() && rational.Sign() > 0 && rational.Num().BitLen() <= 24
}
