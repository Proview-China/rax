package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

type Digest string

func (d Digest) Validate() error {
	value := string(d)
	if !strings.HasPrefix(value, "sha256:") {
		return NewError(ErrorInvalidArgument, ReasonInvalidDigest, "digest must use sha256 prefix")
	}
	raw := strings.TrimPrefix(value, "sha256:")
	decoded, err := hex.DecodeString(raw)
	if err != nil || len(decoded) != sha256.Size {
		return NewError(ErrorInvalidArgument, ReasonInvalidDigest, "digest must contain 32 bytes")
	}
	return nil
}

func DigestJSON(value any) (Digest, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", NewError(ErrorInvalidArgument, ReasonInvalidDigest, "value is not JSON serializable")
	}
	sum := sha256.Sum256(payload)
	return Digest("sha256:" + hex.EncodeToString(sum[:])), nil
}
