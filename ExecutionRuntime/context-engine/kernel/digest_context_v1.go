package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func digestBytesContextV1(ctx context.Context, value []byte) (contract.Digest, error) {
	if err := checkContextV1(ctx); err != nil {
		return "", err
	}
	hash := sha256.New()
	for offset := 0; offset < len(value); offset += StagedCloneChunkBytesV1 {
		if err := checkContextV1(ctx); err != nil {
			return "", err
		}
		end := offset + StagedCloneChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		_, _ = hash.Write(value[offset:end])
	}
	return contract.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), checkContextV1(ctx)
}
