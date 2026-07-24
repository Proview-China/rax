package sqlite

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-host/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func encodeRow(discriminator string, value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", contract.NewError(contract.ErrorInvalidArgument, "canonical_row_invalid", "agent-host sqlite row is invalid or exceeds the canonical bound")
	}
	digest, err := core.CanonicalJSONDigest("praxis.agent-host.sqlite", "1.0.0", discriminator, value)
	if err != nil {
		return nil, "", contract.NewError(contract.ErrorInvalidArgument, "canonical_row_invalid", "agent-host sqlite row digest failed")
	}
	return payload, string(digest), nil
}

func decodeRow[T any](payload []byte, storedDigest, discriminator string) (T, error) {
	var value T
	if err := core.DecodeStrictJSON(payload, &value); err != nil {
		return value, contract.NewError(contract.ErrorConflict, "sqlite_row_invalid", "agent-host sqlite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest("praxis.agent-host.sqlite", "1.0.0", discriminator, value)
	if err != nil || string(digest) != storedDigest {
		return value, contract.NewError(contract.ErrorConflict, "sqlite_row_digest_drift", "agent-host sqlite row digest drifted")
	}
	return value, nil
}
