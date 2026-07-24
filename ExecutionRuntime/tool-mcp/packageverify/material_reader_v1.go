package packageverify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"reflect"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func ReadExactArtifactV1(ctx context.Context, reader runtimeports.SupplyChainArtifactExactReaderV1, ref runtimeports.SupplyChainArtifactContentRefV1, maximum uint64) ([]byte, error) {
	if isNilV1(ctx) || isNilV1(reader) || ref.Validate() != nil || maximum == 0 || ref.Size > maximum {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "exact artifact read request is invalid or exceeds policy")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stream, err := reader.OpenExactSupplyChainArtifactV1(ctx, ref)
	if err != nil {
		return nil, err
	}
	return readAndCloseExactV1(ctx, stream, ref.Digest, ref.Size, maximum)
}

func ReadExactTrustMaterialV1(ctx context.Context, reader runtimeports.SupplyChainTrustMaterialExactReaderV1, ref runtimeports.SupplyChainTrustMaterialRefV1, maximum uint64) ([]byte, error) {
	if isNilV1(ctx) || isNilV1(reader) || ref.Validate() != nil || maximum == 0 {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "exact trust material read request is invalid")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stream, err := reader.OpenExactSupplyChainTrustMaterialV1(ctx, ref)
	if err != nil {
		return nil, err
	}
	return readAndCloseExactV1(ctx, stream, ref.Digest, 0, maximum)
}

func ReadExactTrustPolicyDocumentV1(ctx context.Context, reader runtimeports.SupplyChainTrustPolicyDocumentExactReaderV1, ref runtimeports.SupplyChainTrustPolicyDocumentRefV1, maximum uint64) ([]byte, error) {
	if isNilV1(ctx) || isNilV1(reader) || ref.Validate() != nil || maximum == 0 || ref.Size > maximum {
		return nil, core.NewError(core.ErrorInvalidArgument, core.ReasonCanonicalLimitExceeded, "exact trust policy document read request is invalid or exceeds policy")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	stream, err := reader.OpenExactSupplyChainTrustPolicyDocumentV1(ctx, ref)
	if err != nil {
		return nil, err
	}
	return readAndCloseExactV1(ctx, stream, ref.Digest, ref.Size, maximum)
}

func readAndCloseExactV1(ctx context.Context, stream io.ReadCloser, expected core.Digest, exactSize, maximum uint64) ([]byte, error) {
	if isNilV1(stream) {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceSourceMissing, "exact content reader returned nil stream")
	}
	data, readErr := io.ReadAll(io.LimitReader(stream, int64(maximum)+1))
	closeErr := stream.Close()
	if readErr != nil {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceSourceMissing, "exact content stream read failed")
	}
	if closeErr != nil {
		return nil, core.NewError(core.ErrorUnavailable, core.ReasonEvidenceSourceMissing, "exact content stream close failed")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if uint64(len(data)) > maximum || (exactSize != 0 && uint64(len(data)) != exactSize) {
		return nil, core.NewError(core.ErrorConflict, core.ReasonCanonicalLimitExceeded, "exact content size drifted")
	}
	sum := sha256.Sum256(data)
	digest := core.Digest("sha256:" + hex.EncodeToString(sum[:]))
	if digest != expected {
		return nil, core.NewError(core.ErrorConflict, core.ReasonInvalidDigest, "exact content digest drifted")
	}
	return append([]byte(nil), data...), nil
}

func isNilV1(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
