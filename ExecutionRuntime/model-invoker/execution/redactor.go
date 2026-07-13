package execution

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/union"
)

type EventSanitizer interface {
	Sanitize(union.UnifiedExecutionEvent) (union.UnifiedExecutionEvent, error)
}

type EventRedactor struct {
	literals [][]byte
}

func NewEventRedactor(literals ...[]byte) (*EventRedactor, error) {
	redactor := &EventRedactor{literals: make([][]byte, 0, len(literals))}
	for _, literal := range literals {
		if len(literal) == 0 {
			return nil, fmt.Errorf("%w: redaction literal is empty", ErrInvalidInvocation)
		}
		redactor.literals = append(redactor.literals, append([]byte(nil), literal...))
	}
	return redactor, nil
}

var sensitiveEventKeySuffixes = []string{
	"accesstoken", "apikey", "authorization", "clientsecret", "credentials", "credential",
	"oauthtoken", "password", "refreshtoken", "secret",
}

func (redactor *EventRedactor) Sanitize(event union.UnifiedExecutionEvent) (union.UnifiedExecutionEvent, error) {
	clone, err := event.Clone()
	if err != nil {
		return union.UnifiedExecutionEvent{}, err
	}
	if clone.Model != nil {
		for index := range clone.Model.Content {
			clone.Model.Content[index].Text = redactor.redactString(clone.Model.Content[index].Text)
			clone.Model.Content[index].Reference = redactor.redactString(clone.Model.Content[index].Reference)
			clone.Model.Content[index].JSON, err = redactor.redactRaw(clone.Model.Content[index].JSON)
			if err != nil {
				return union.UnifiedExecutionEvent{}, err
			}
			for key, value := range clone.Model.Content[index].Metadata {
				clone.Model.Content[index].Metadata[key] = redactor.redactString(value)
			}
		}
		clone.Model.Payload, err = redactor.redactRaw(clone.Model.Payload)
	}
	if err == nil && clone.Item != nil {
		clone.Item.Item.Payload, err = redactor.redactRaw(clone.Item.Item.Payload)
		if err == nil {
			clone.Item.Delta, err = redactor.redactRaw(clone.Item.Delta)
		}
	}
	if err == nil && clone.Control != nil {
		clone.Control.Payload, err = redactor.redactRaw(clone.Control.Payload)
	}
	if err == nil && clone.Diagnostic != nil {
		clone.Diagnostic.Message = redactor.redactString(clone.Diagnostic.Message)
		clone.Diagnostic.Payload, err = redactor.redactRaw(clone.Diagnostic.Payload)
	}
	if err != nil {
		return union.UnifiedExecutionEvent{}, err
	}
	return clone, nil
}

func (redactor *EventRedactor) redactRaw(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("event payload contains multiple JSON values")
	}
	value = redactor.redactValue(value)
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

func (redactor *EventRedactor) redactValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if isSensitiveEventKey(key) {
				typed[key] = "[REDACTED]"
				continue
			}
			typed[key] = redactor.redactValue(child)
		}
		return typed
	case []any:
		for index := range typed {
			typed[index] = redactor.redactValue(typed[index])
		}
		return typed
	case string:
		return redactor.redactString(typed)
	default:
		return value
	}
}

func isSensitiveEventKey(key string) bool {
	var normalized strings.Builder
	for _, character := range strings.ToLower(strings.TrimSpace(key)) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			normalized.WriteRune(character)
		}
	}
	value := normalized.String()
	for _, suffix := range sensitiveEventKeySuffixes {
		if strings.HasSuffix(value, suffix) {
			return true
		}
	}
	return false
}

func (redactor *EventRedactor) redactString(value string) string {
	payload := []byte(value)
	for _, literal := range redactor.literals {
		payload = bytes.ReplaceAll(payload, literal, []byte("[REDACTED]"))
	}
	return string(payload)
}
