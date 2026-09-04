package parse

import (
	"time"

	"github.com/go-errors/errors"
)

func MsDuration(milliseconds *int, fallback time.Duration, field string) (time.Duration, error) {
	if milliseconds == nil {
		return fallback, nil
	}
	if *milliseconds <= 0 {
		return 0, errors.Errorf(
			"%s: must be a positive duration in ms, got %d",
			field,
			*milliseconds,
		)
	}
	return time.Duration(*milliseconds) * time.Millisecond, nil
}

func Duration(value string, fallback time.Duration, field string) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, errors.Errorf("%s: invalid duration %q: %w", field, value, err)
	}
	if duration <= 0 {
		return 0, errors.Errorf("%s: duration must be positive, got %q", field, value)
	}
	return duration, nil
}
