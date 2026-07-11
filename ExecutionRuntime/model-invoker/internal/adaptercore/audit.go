package adaptercore

import (
	"encoding/json"
	"fmt"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
)

// MarshalAuditRequest serializes a provider SDK request into the controlled
// audit payload used by model-invoker. Authentication data must not be part of
// value. Streaming is represented explicitly because several SDK request
// structs keep it outside their public parameter type.
func MarshalAuditRequest(value any, stream bool) (modelinvoker.RawPayload, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return modelinvoker.RawPayload{}, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return modelinvoker.RawPayload{}, fmt.Errorf("audit request must serialize as a JSON object")
	}
	if stream {
		object["stream"] = json.RawMessage("true")
	}
	raw, err = json.Marshal(object)
	if err != nil {
		return modelinvoker.RawPayload{}, err
	}
	return modelinvoker.NewRawPayload(raw), nil
}

// RawPayload prefers the SDK's exact JSON and falls back to a deterministic
// serialization of the decoded value.
func RawPayload(raw string, fallback any) (modelinvoker.RawPayload, error) {
	if raw != "" {
		return modelinvoker.NewRawPayload([]byte(raw)), nil
	}
	data, err := json.Marshal(fallback)
	if err != nil {
		return modelinvoker.RawPayload{}, err
	}
	return modelinvoker.NewRawPayload(data), nil
}
