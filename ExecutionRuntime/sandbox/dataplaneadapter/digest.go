package dataplaneadapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func canonicalJSON(data []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

func canonicalDigest(kind string, value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte(ContractVersionV1))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(kind))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(data)
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || value[:len("sha256:")] != "sha256:" {
		return false
	}
	_, err := hex.DecodeString(value[len("sha256:"):])
	return err == nil
}

func validEffectKind(value string) bool {
	switch value {
	case "praxis.sandbox/backend-discovery", "praxis.sandbox/allocate", "praxis.sandbox/activate", "praxis.sandbox/open", "praxis.sandbox/cancel", "praxis.sandbox/close", "praxis.sandbox/fence", "praxis.sandbox/release", "praxis.sandbox/inspect", "praxis.sandbox/cleanup", "praxis.sandbox/workspace-commit", CheckpointEffectKindV1:
		return true
	default:
		return false
	}
}
