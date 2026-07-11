package modelinvoker

import "encoding/json"

const redactedPayload = "[REDACTED]"

// RawPayload keeps an audit payload out of ordinary formatting and JSON logs.
// Bytes is an explicit privileged read and always returns a defensive copy.
type RawPayload struct {
	data []byte
}

func NewRawPayload(data []byte) RawPayload {
	return RawPayload{data: append([]byte(nil), data...)}
}

func (r RawPayload) Bytes() []byte {
	return append([]byte(nil), r.data...)
}

func (r RawPayload) Len() int {
	return len(r.data)
}

func (r RawPayload) Empty() bool {
	return len(r.data) == 0
}

func (RawPayload) String() string {
	return redactedPayload
}

func (RawPayload) GoString() string {
	return redactedPayload
}

func (RawPayload) MarshalJSON() ([]byte, error) {
	return json.Marshal(redactedPayload)
}

func (RawPayload) MarshalText() ([]byte, error) {
	return []byte(redactedPayload), nil
}
