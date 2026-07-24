package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func DecodeValidatePromptAssetEngineeringRequestV1(ctx context.Context, payload []byte) (ValidatePromptAssetEngineeringRequestV1, error) {
	return decodeEngineeringRequestV1[ValidatePromptAssetEngineeringRequestV1](ctx, payload, EngineeringValidatePromptAssetV1)
}

func DecodePreviewPromptCandidatesEngineeringRequestV1(ctx context.Context, payload []byte) (PreviewPromptCandidatesEngineeringRequestV1, error) {
	return decodeEngineeringRequestV1[PreviewPromptCandidatesEngineeringRequestV1](ctx, payload, EngineeringPreviewPromptV1)
}

func DecodePrepareContextEvaluationRequestV1(ctx context.Context, payload []byte) (PrepareContextEvaluationRequestV1, error) {
	return decodeEngineeringRequestV1[PrepareContextEvaluationRequestV1](ctx, payload, EngineeringPrepareEvaluationV1)
}

func DecodeAdmitContextEvaluationRequestV1(ctx context.Context, payload []byte) (AdmitContextEvaluationRequestV1, error) {
	return decodeEngineeringRequestV1[AdmitContextEvaluationRequestV1](ctx, payload, EngineeringAdmitEvaluationV1)
}

func DecodeBuildContextFeedbackRequestV1(ctx context.Context, payload []byte) (BuildContextFeedbackRequestV1, error) {
	return decodeEngineeringRequestV1[BuildContextFeedbackRequestV1](ctx, payload, EngineeringBuildFeedbackV1)
}

func EncodeValidatePromptAssetEngineeringRequestV1(ctx context.Context, request ValidatePromptAssetEngineeringRequestV1) ([]byte, error) {
	return encodeEngineeringRequestV1(ctx, request, SealValidatePromptAssetEngineeringRequestV1)
}

func EncodePreviewPromptCandidatesEngineeringRequestV1(ctx context.Context, request PreviewPromptCandidatesEngineeringRequestV1) ([]byte, error) {
	return encodeEngineeringRequestV1(ctx, request, SealPreviewPromptCandidatesEngineeringRequestV1)
}

func EncodePrepareContextEvaluationRequestV1(ctx context.Context, request PrepareContextEvaluationRequestV1) ([]byte, error) {
	return encodeEngineeringRequestV1(ctx, request, SealPrepareContextEvaluationRequestV1)
}

func EncodeAdmitContextEvaluationRequestV1(ctx context.Context, request AdmitContextEvaluationRequestV1) ([]byte, error) {
	return encodeEngineeringRequestV1(ctx, request, SealAdmitContextEvaluationRequestV1)
}

func EncodeBuildContextFeedbackRequestV1(ctx context.Context, request BuildContextFeedbackRequestV1) ([]byte, error) {
	return encodeEngineeringRequestV1(ctx, request, SealBuildContextFeedbackRequestV1)
}

func EncodeValidatePromptAssetEngineeringResponseV1(ctx context.Context, response ValidatePromptAssetEngineeringResponseV1) ([]byte, error) {
	return encodeEngineeringResponseV1(ctx, response, response.Meta, response.limits)
}

func EncodePreviewPromptCandidatesEngineeringResponseV1(ctx context.Context, response PreviewPromptCandidatesEngineeringResponseV1) ([]byte, error) {
	return encodeEngineeringResponseV1(ctx, response, response.Meta, response.limits)
}

func EncodePrepareContextEvaluationResponseV1(ctx context.Context, response PrepareContextEvaluationResponseV1) ([]byte, error) {
	return encodeEngineeringResponseV1(ctx, response, response.Meta, response.limits)
}

func EncodeAdmitContextEvaluationResponseV1(ctx context.Context, response AdmitContextEvaluationResponseV1) ([]byte, error) {
	return encodeEngineeringResponseV1(ctx, response, response.Meta, response.limits)
}

func EncodeBuildContextFeedbackResponseV1(ctx context.Context, response BuildContextFeedbackResponseV1) ([]byte, error) {
	return encodeEngineeringResponseV1(ctx, response, response.Meta, response.limits)
}

func decodeEngineeringRequestV1[T any](ctx context.Context, payload []byte, op ContextEngineeringOperationV1) (T, error) {
	var zero T
	meta, err := preflightEngineeringMetaV1(ctx, payload, op)
	if err != nil {
		return zero, err
	}
	copyPayload, err := cloneCodecBytesV1(ctx, payload)
	if err != nil {
		return zero, mapEngineeringErrorV1(op, "payload", err)
	}
	scan, err := scanDuplicateKeysContextV1(ctx, copyPayload, "", OfflineSDKLimitsV1{})
	if err != nil {
		if ctx.Err() != nil {
			return zero, mapEngineeringErrorV1(op, "payload", ctx.Err())
		}
		if errors.Is(err, contract.ErrLimitExceeded) {
			return zero, mapEngineeringErrorV1(op, "payload", err)
		}
		return zero, mapEngineeringErrorV1(op, "payload", errors.Join(contract.ErrInvalid, err))
	}
	if err := validateEngineeringWireScanV1(scan, meta.Limits, op); err != nil {
		return zero, err
	}
	decoder := json.NewDecoder(&contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(copyPayload)})
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&zero); err != nil {
		if ctx.Err() != nil {
			return *new(T), mapEngineeringErrorV1(op, "payload", ctx.Err())
		}
		return *new(T), mapEngineeringErrorV1(op, "payload", errors.Join(contract.ErrInvalid, err))
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return *new(T), mapEngineeringErrorV1(op, "payload", contract.ErrInvalid)
	}
	requestMeta, ok := engineeringMetaFromRequestV1(zero)
	if !ok || requestMeta != meta {
		return *new(T), mapEngineeringErrorV1(op, "meta", contract.ErrConflict)
	}
	if err := validateEngineeringRequestV1(ctx, op, zero, meta); err != nil {
		return *new(T), err
	}
	return zero, nil
}

func preflightEngineeringMetaV1(ctx context.Context, payload []byte, op ContextEngineeringOperationV1) (ContextEngineeringRequestMetaV1, error) {
	if err := engineeringContextErrV1(ctx); err != nil {
		return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "context", err)
	}
	if uint64(len(payload)) > hardEngineeringWireBytesV1 {
		return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "payload", contract.ErrLimitExceeded)
	}
	start, end, err := findTopLevelJSONFieldV1(ctx, payload, "meta")
	if err != nil || start < 0 {
		return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "meta", contract.ErrInvalid)
	}
	var meta ContextEngineeringRequestMetaV1
	decoder := json.NewDecoder(&contextChunkReaderV1{ctx: ctx, reader: bytes.NewReader(payload[start:end])})
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&meta); err != nil {
		if ctx.Err() != nil {
			return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "meta", ctx.Err())
		}
		return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "meta", contract.ErrInvalid)
	}
	if err := validateEngineeringMetaV1(meta, op); err != nil {
		return ContextEngineeringRequestMetaV1{}, err
	}
	if uint64(len(payload)) > meta.Limits.MaxWireBytes {
		return ContextEngineeringRequestMetaV1{}, mapEngineeringErrorV1(op, "payload", contract.ErrLimitExceeded)
	}
	return meta, nil
}

func validateEngineeringWireScanV1(scan strictJSONScanV1, limits ContextEngineeringLimitsV1, op ContextEngineeringOperationV1) error {
	var nestedRefs, evidence uint64
	for path, count := range scan.arrayCounts {
		if strings.HasSuffix(path, ".fragments") && count > uint64(limits.MaxPromptFragments) {
			return mapEngineeringErrorV1(op, path, contract.ErrLimitExceeded)
		}
		if strings.HasSuffix(path, ".outcomes") && count > uint64(limits.MaxOutcomes) {
			return mapEngineeringErrorV1(op, path, contract.ErrLimitExceeded)
		}
		if strings.HasSuffix(path, ".evidence") || strings.HasSuffix(path, ".user_correction_evidence") {
			evidence += count
		}
		if strings.HasSuffix(path, "_refs") || strings.HasSuffix(path, ".render_compatibility") || strings.HasSuffix(path, ".evidence") || strings.HasSuffix(path, ".user_correction_evidence") {
			nestedRefs += count
		}
		if evidence > uint64(limits.MaxEvidenceRefs) || nestedRefs > uint64(limits.MaxNestedRefs) {
			return mapEngineeringErrorV1(op, path, contract.ErrLimitExceeded)
		}
	}
	return nil
}

func encodeEngineeringRequestV1[T any](ctx context.Context, request T, seal func(context.Context, T) (T, error)) ([]byte, error) {
	sealed, err := seal(ctx, request)
	if err != nil {
		return nil, err
	}
	meta, ok := engineeringMetaFromRequestV1(sealed)
	if !ok {
		return nil, mapEngineeringErrorV1("", "request", contract.ErrInvalid)
	}
	buffer := &boundedCodecBufferV1{ctx: ctx, max: meta.Limits.MaxWireBytes}
	if err := buffer.writeJSON(sealed); err != nil {
		return nil, mapEngineeringErrorV1(meta.Operation, "request", err)
	}
	return cloneCodecBytesV1(ctx, buffer.buf.Bytes())
}

func encodeEngineeringResponseV1[T any](ctx context.Context, response T, meta ContextEngineeringResponseMetaV1, limits ContextEngineeringLimitsV1) ([]byte, error) {
	if err := validateEngineeringResponseV1(ctx, response, meta, limits); err != nil {
		return nil, err
	}
	buffer := &boundedCodecBufferV1{ctx: ctx, max: limits.MaxWireBytes}
	if err := buffer.writeJSON(response); err != nil {
		return nil, mapEngineeringErrorV1(meta.Operation, "response", err)
	}
	return cloneCodecBytesV1(ctx, buffer.buf.Bytes())
}

func validateEngineeringResponseV1(ctx context.Context, response any, meta ContextEngineeringResponseMetaV1, limits ContextEngineeringLimitsV1) error {
	if err := engineeringContextErrV1(ctx); err != nil {
		return mapEngineeringErrorV1(meta.Operation, "context", err)
	}
	if meta.ContractVersion != ContextEngineeringSDKContractVersionV1 || !engineeringIDV1(meta.RequestID) || meta.Operation.Validate() != nil || meta.RequestDigest.Validate() != nil || meta.ResultDigest.Validate() != nil || limits.Validate() != nil {
		return mapEngineeringErrorV1(meta.Operation, "meta", contract.ErrInvalid)
	}
	if err := validateEngineeringResponseShapeV1(response); err != nil {
		return mapEngineeringErrorV1(meta.Operation, "response", err)
	}
	want, err := engineeringResponseDigestV1(ctx, response, limits.MaxCanonicalBytes)
	if err != nil {
		return mapEngineeringErrorV1(meta.Operation, "meta.result_digest", err)
	}
	if want != meta.ResultDigest {
		return mapEngineeringErrorV1(meta.Operation, "meta.result_digest", contract.ErrConflict)
	}
	return nil
}

func engineeringResponseDigestV1(ctx context.Context, response any, max uint64) (contract.Digest, error) {
	switch value := response.(type) {
	case ValidatePromptAssetEngineeringResponseV1:
		value.Meta.ResultDigest = ""
		return engineeringCanonicalDigestV1(ctx, "response:"+string(value.Meta.Operation), value, max)
	case PreviewPromptCandidatesEngineeringResponseV1:
		value.Meta.ResultDigest = ""
		return engineeringCanonicalDigestV1(ctx, "response:"+string(value.Meta.Operation), value, max)
	case PrepareContextEvaluationResponseV1:
		value.Meta.ResultDigest = ""
		return engineeringCanonicalDigestV1(ctx, "response:"+string(value.Meta.Operation), value, max)
	case AdmitContextEvaluationResponseV1:
		value.Meta.ResultDigest = ""
		return engineeringCanonicalDigestV1(ctx, "response:"+string(value.Meta.Operation), value, max)
	case BuildContextFeedbackResponseV1:
		value.Meta.ResultDigest = ""
		return engineeringCanonicalDigestV1(ctx, "response:"+string(value.Meta.Operation), value, max)
	default:
		return "", contract.ErrInvalid
	}
}

func validateEngineeringResponseShapeV1(response any) error {
	switch value := response.(type) {
	case ValidatePromptAssetEngineeringResponseV1:
		if value.Valid {
			if value.AssetRef == nil || value.AssetRef.Validate() != nil || len(value.Diagnostics) != 0 {
				return contract.ErrConflict
			}
		} else if value.AssetRef != nil || len(value.Diagnostics) == 0 {
			return contract.ErrConflict
		}
	case PreviewPromptCandidatesEngineeringResponseV1:
		return value.Candidates.Validate()
	case PrepareContextEvaluationResponseV1:
		return value.Input.Validate()
	case AdmitContextEvaluationResponseV1:
		digest, err := value.Evaluation.DigestValue()
		if err != nil || value.EvaluationRef != (contract.FactRef{ID: value.Evaluation.ID, Revision: value.Evaluation.Revision, Digest: digest}) {
			return contract.ErrConflict
		}
	case BuildContextFeedbackResponseV1:
		digest, err := value.Feedback.DigestValue()
		if err != nil || value.FeedbackRef != (contract.FactRef{ID: value.Feedback.ID, Revision: value.Feedback.Revision, Digest: digest}) {
			return contract.ErrConflict
		}
	default:
		return contract.ErrInvalid
	}
	return nil
}

func engineeringMetaFromRequestV1(value any) (ContextEngineeringRequestMetaV1, bool) {
	switch request := value.(type) {
	case ValidatePromptAssetEngineeringRequestV1:
		return request.Meta, true
	case PreviewPromptCandidatesEngineeringRequestV1:
		return request.Meta, true
	case PrepareContextEvaluationRequestV1:
		return request.Meta, true
	case AdmitContextEvaluationRequestV1:
		return request.Meta, true
	case BuildContextFeedbackRequestV1:
		return request.Meta, true
	default:
		return ContextEngineeringRequestMetaV1{}, false
	}
}
