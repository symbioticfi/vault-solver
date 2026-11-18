package observability

import (
	"fmt"

	"go.uber.org/zap/zapcore"
)

type redactingCore struct{ zapcore.Core }

func (c *redactingCore) With(fields []zapcore.Field) zapcore.Core {
	return &redactingCore{c.Core.With(redactLogFields(fields))}
}

func (c *redactingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	entry.Message = redactURLs(entry.Message)
	checked = c.Core.Check(entry, checked)
	if checked == nil {
		return nil
	}
	// Keep the sinks' checks/sampler and zap's ErrorOutput on the original CheckedEntry.
	return checked.Before(entry, func(e zapcore.Entry, fields []zapcore.Field) (zapcore.Entry, []zapcore.Field) {
		e.Message = redactURLs(e.Message)
		return e, redactLogFields(fields)
	})
}

func redactLogFields(fields []zapcore.Field) []zapcore.Field {
	clean := append([]zapcore.Field(nil), fields...)
	for i := range clean {
		field := &clean[i]
		if field.Type == zapcore.StringType {
			field.String = redactURLs(field.String)
			continue
		}
		if field.Type == zapcore.ErrorType {
			if err, ok := field.Interface.(error); ok && err != nil {
				field.Interface = redactedLogError{cause: err}
			}
			continue
		}
		if field.Type == zapcore.StringerType {
			if value, ok := field.Interface.(fmt.Stringer); ok {
				field.Type = zapcore.StringType
				field.String = redactURLs(value.String())
				field.Interface = nil
			}
		}
	}
	return clean
}

// Only display is changed; errors.As can still recover backend grouping diagnostics.
type redactedLogError struct{ cause error }

func (e redactedLogError) Error() string { return redactURLs(e.cause.Error()) }
func (e redactedLogError) Unwrap() error { return e.cause }
