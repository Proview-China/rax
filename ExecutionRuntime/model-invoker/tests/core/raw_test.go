package core_test

import (
	"encoding"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	. "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

const testRedactedPayload = "[REDACTED]"

func TestRawPayloadDefensivelyCopiesInputAndOutput(t *testing.T) {
	source := []byte(`{"secret":"test-api-key"}`)
	payload := NewRawPayload(source)
	source[2] = 'X'

	first := payload.Bytes()
	if got, want := string(first), `{"secret":"test-api-key"}`; got != want {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
	first[2] = 'Y'
	if got, want := string(payload.Bytes()), `{"secret":"test-api-key"}`; got != want {
		t.Fatalf("second Bytes() = %q, want %q", got, want)
	}
	if payload.Len() != len(`{"secret":"test-api-key"}`) {
		t.Fatalf("Len() = %d", payload.Len())
	}
	if payload.Empty() {
		t.Fatal("Empty() = true for non-empty payload")
	}
}

func TestRawPayloadDefaultFormattingAndJSONAreRedacted(t *testing.T) {
	const secret = "test-api-key"
	payload := NewRawPayload([]byte(`{"token":"` + secret + `"}`))

	formats := map[string]string{
		"String":     payload.String(),
		"GoString":   payload.GoString(),
		"percent-s":  fmt.Sprintf("%s", payload),
		"percent-v":  fmt.Sprintf("%v", payload),
		"percent-go": fmt.Sprintf("%#v", payload),
	}
	for name, formatted := range formats {
		t.Run(name, func(t *testing.T) {
			if formatted != testRedactedPayload {
				t.Fatalf("formatted payload = %q, want %q", formatted, testRedactedPayload)
			}
			if strings.Contains(formatted, secret) {
				t.Fatalf("formatted payload leaked secret: %q", formatted)
			}
		})
	}

	encoded, err := json.Marshal(struct {
		Raw RawPayload `json:"raw"`
	}{Raw: payload})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if got, want := string(encoded), `{"raw":"[REDACTED]"}`; got != want {
		t.Fatalf("json.Marshal() = %s, want %s", got, want)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("JSON leaked secret: %s", encoded)
	}

	marshaler, ok := any(payload).(encoding.TextMarshaler)
	if !ok {
		t.Fatal("RawPayload does not implement encoding.TextMarshaler")
	}
	text, err := marshaler.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText() error = %v", err)
	}
	if got := string(text); got != testRedactedPayload {
		t.Fatalf("MarshalText() = %q, want %q", got, testRedactedPayload)
	}
}

func TestRawPayloadZeroValueRemainsSafeAndUsable(t *testing.T) {
	var payload RawPayload
	if !payload.Empty() || payload.Len() != 0 {
		t.Fatalf("zero RawPayload: Empty()=%v Len()=%d", payload.Empty(), payload.Len())
	}
	if got := payload.Bytes(); len(got) != 0 {
		t.Fatalf("zero Bytes() = %#v, want empty", got)
	}
	if got := payload.String(); got != testRedactedPayload {
		t.Fatalf("zero String() = %q, want %q", got, testRedactedPayload)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal(zero) error = %v", err)
	}
	if got, want := string(encoded), `"[REDACTED]"`; got != want {
		t.Fatalf("json.Marshal(zero) = %s, want %s", got, want)
	}
}

func TestRawPayloadConcurrentExplicitReadsReturnIndependentCopies(t *testing.T) {
	payload := NewRawPayload([]byte("immutable-audit-payload"))
	const readers = 64
	done := make(chan struct{}, readers)
	for index := 0; index < readers; index++ {
		go func(index int) {
			copy := payload.Bytes()
			copy[0] = byte('A' + index%26)
			done <- struct{}{}
		}(index)
	}
	for index := 0; index < readers; index++ {
		<-done
	}
	if got, want := string(payload.Bytes()), "immutable-audit-payload"; got != want {
		t.Fatalf("payload changed after concurrent reads: %q, want %q", got, want)
	}
}
