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
		return errors.Errorf("3f invalid or ambiguous JSON response at byte %d: %w", d.InputOffset(), err)
	}
	if _, err := d.Token(); err != io.EOF {
		return errors.Errorf("3f trailing JSON response data at byte %d", d.InputOffset())
	}
	return nil
}

func checkValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, errors.New("JSON nesting exceeds 64 levels")
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
		return nil, errors.Errorf("unexpected JSON delimiter %q", delim)
	}
	seen := make(map[string]bool)
	for d.More() {
		keyToken, keyErr := d.Token()
		key, ok := keyToken.(string)
		switch {
		case keyErr != nil:
			return nil, errors.Errorf("JSON property name: %w", keyErr)
		case !ok:
			return nil, errors.Errorf("JSON property name is a %T, not a string", keyToken)
		case seen[key]:
			return nil, errors.Errorf("duplicate JSON property %.64q", key)
		}
		seen[key] = true
		value, valueErr := checkValue(d, depth+1)
		if valueErr != nil {
			return nil, valueErr
		}
		if key == "id" || key == "auctionId" || key == "offerId" {
			number, isNumber := value.(json.Number)
			if !isNumber {
				return nil, errors.Errorf("JSON %s is a %T, not a number", key, value)
			}
			if !exactIDNumber(number) {
				return nil, errors.Errorf("JSON %s %.64s is not an exact positive integer below 16777216", key, number)
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
