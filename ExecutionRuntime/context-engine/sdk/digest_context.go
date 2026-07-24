package sdk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/kernel"
)

func digestJSONContextV1(ctx context.Context, value any) (contract.Digest, error) {
	hash := sha256.New()
	if err := writeJSONContextV1(ctx, hash, value); err != nil {
		return "", err
	}
	return contract.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), ctx.Err()
}

func digestBytesContextV1(ctx context.Context, value []byte) (contract.Digest, error) {
	if ctx == nil {
		return "", contract.ErrInvalid
	}
	hash := sha256.New()
	for offset := 0; offset < len(value); offset += kernel.StagedCloneChunkBytesV1 {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		end := offset + kernel.StagedCloneChunkBytesV1
		if end > len(value) {
			end = len(value)
		}
		_, _ = hash.Write(value[offset:end])
	}
	return contract.Digest("sha256:" + hex.EncodeToString(hash.Sum(nil))), ctx.Err()
}
