package sqlite

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func marshalStrict(value any) ([]byte, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "runtime Binding sqlite canonical row is invalid or too large")
	}
	return payload, nil
}

func digestRow(discriminator string, value any) (core.Digest, error) {
	return core.CanonicalJSONDigest("praxis.runtime.binding.sqlite", "v1", discriminator, value)
}

func decodeRow[T any](payload []byte, storedDigest string, discriminator string) (T, error) {
	var value T
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite row is not strict canonical JSON")
	}
	digest, err := digestRow(discriminator, value)
	if err != nil || string(digest) != storedDigest {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "runtime Binding sqlite row digest drifted")
	}
	return value, nil
}
