package parse

import (
	"encoding/json"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

type Object map[string]json.RawMessage

func DecodeObject(payload []byte) (Object, error) {
	var value Object
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "platform webhook payload must be an object")
	}
	return value, nil
}
func ObjectField(value Object, key string) (Object, error) {
	raw, ok := value[key]
	if !ok {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook object field is missing")
	}
	return DecodeObject(raw)
}
func String(value Object, key string) (string, error) {
	raw, ok := value[key]
	if !ok {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook string field is missing")
	}
	var result string
	if err := core.DecodeStrictJSON(raw, &result); err != nil || strings.TrimSpace(result) == "" || len(result) > 4096 {
		return "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook string field is invalid")
	}
	return result, nil
}
func OptionalString(value Object, key string) string {
	raw, ok := value[key]
	if !ok {
		return ""
	}
	var result string
	if core.DecodeStrictJSON(raw, &result) != nil || len(result) > 4096 {
		return ""
	}
	return result
}
func Int64(value Object, key string) (int64, error) {
	raw, ok := value[key]
	if !ok {
		return 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook integer field is missing")
	}
	var result int64
	if err := core.DecodeStrictJSON(raw, &result); err != nil || result <= 0 {
		return 0, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook integer field is invalid")
	}
	return result, nil
}
func FirstObject(value Object, key string) (Object, error) {
	raw, ok := value[key]
	if !ok {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "platform webhook array field is missing")
	}
	var values []json.RawMessage
	if err := core.DecodeStrictJSON(raw, &values); err != nil || len(values) != 1 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "platform webhook action array must contain exactly one item")
	}
	return DecodeObject(values[0])
}
