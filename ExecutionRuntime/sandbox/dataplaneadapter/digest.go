package dataplaneadapter

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
)

func canonicalJSON(data []byte) ([]byte, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("canonical JSON contains trailing data")
		}
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
	case "praxis.sandbox/backend-discovery", "praxis.sandbox/allocate", "praxis.sandbox/activate", "praxis.sandbox/open", "praxis.sandbox/cancel", "praxis.sandbox/close", "praxis.sandbox/fence", "praxis.sandbox/release", "praxis.sandbox/inspect", "praxis.sandbox/cleanup", "praxis.sandbox/workspace-commit", "praxis.sandbox/workspace-read", CheckpointEffectKindV1:
		return true
	default:
		return false
	}
}
