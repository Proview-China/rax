package sqlite

import (
	"encoding/json"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

const rowDomainV1 = "praxis.harness.session-event.sqlite"

func encodeRowV1(discriminator string, value any) ([]byte, string, error) {
	payload, err := json.Marshal(value)
	if err != nil || len(payload) == 0 || len(payload) > core.MaxCanonicalDocumentBytes {
		return nil, "", invalidV1("Harness SQLite row exceeds its canonical bound")
	}
	digest, err := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", discriminator, value)
	if err != nil {
		return nil, "", invalidV1("Harness SQLite row cannot be canonicalized")
	}
	return payload, string(digest), nil
}

func decodeRowV1[T any](payload []byte, storedDigest, discriminator string) (T, error) {
	var value T
	if len(payload) == 0 || storedDigest == "" || core.DecodeStrictJSON(payload, &value) != nil {
		return value, corruptV1("Harness SQLite row is not strict canonical JSON")
	}
	digest, err := core.CanonicalJSONDigest(rowDomainV1, "1.0.0", discriminator, value)
	if err != nil || string(digest) != storedDigest {
		return value, corruptV1("Harness SQLite row digest drifted")
	}
	return value, nil
}
