package effect

import (
	"bytes"
	"fmt"
	"sort"
)

var redactedMarker = []byte("[REDACTED]")

// ContentRedactor is applied before captured file bytes enter a public diff.
// Implementations must be deterministic and must not expose the values they
// match through formatting or errors.
type ContentRedactor interface {
	Redact([]byte) []byte
}

type LiteralRedactor struct {
	values [][]byte
}

// NewLiteralRedactor defensively copies literal sensitive values. Empty
// values are rejected because they would match every byte boundary.
func NewLiteralRedactor(values ...[]byte) (*LiteralRedactor, error) {
	redactor := &LiteralRedactor{values: make([][]byte, 0, len(values))}
	for _, value := range values {
		if len(value) == 0 {
			return nil, fmt.Errorf("%w: redaction literal is empty", ErrInvalidPolicy)
		}
		redactor.values = append(redactor.values, append([]byte(nil), value...))
	}
	// Longest-first prevents a shorter prefix from leaving a suffix behind.
	sort.Slice(redactor.values, func(i, j int) bool { return len(redactor.values[i]) > len(redactor.values[j]) })
	return redactor, nil
}

func (redactor *LiteralRedactor) Redact(payload []byte) []byte {
	result := append([]byte(nil), payload...)
	if redactor == nil {
		return result
	}
	for _, value := range redactor.values {
		result = bytes.ReplaceAll(result, value, redactedMarker)
	}
	return result
}

type passthroughRedactor struct{}

func (passthroughRedactor) Redact(payload []byte) []byte { return append([]byte(nil), payload...) }
