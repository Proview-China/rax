package assemblypublication

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func encodePublicationRowV2(discriminator string, value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "Assembly publication SQLite row is invalid or exceeds the canonical bound")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.assembly-publication.sqlite", "2.0.0", discriminator, value)
	if err != nil {
		return nil, "", core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidCanonicalForm, "Assembly publication SQLite row digest failed")
	}
	return payload, string(digest), nil
}

func decodePublicationRowV2[T any](payload []byte, storedDigest, discriminator string) (T, error) {
	var value T
	if len(payload) == 0 || storedDigest == "" || core.DecodeStrictJSON(payload, &value) != nil {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidCanonicalForm, "Assembly publication SQLite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest("praxis.harness.assembly-publication.sqlite", "2.0.0", discriminator, value)
	if err != nil || string(digest) != storedDigest {
		return value, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "Assembly publication SQLite row digest drifted")
	}
	return value, nil
}
