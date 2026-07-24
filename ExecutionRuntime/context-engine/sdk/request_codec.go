package sdk

import (
	"context"
	"encoding/base64"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func SealValidateRecipeRequestV1(ctx context.Context, request ValidateRecipeRequestV1) (ValidateRecipeRequestV1, error) {
	const op = OfflineValidateRecipeV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	if err := preflightValidateRecipeRequestV1(request); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	request = cloneValidateRequestV1(request)
	digest, err := validateRecipeRequestDigestValueV1(request, ctx)
	if err != nil {
		return ValidateRecipeRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return ValidateRecipeRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func SealCompileFrameRequestV1(ctx context.Context, request CompileFrameRequestV1) (CompileFrameRequestV1, error) {
	const op = OfflineCompileFrameV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return CompileFrameRequestV1{}, err
	}
	if err := preflightCompileFrameRequestV1(request); err != nil {
		return CompileFrameRequestV1{}, err
	}
	request, err := cloneCompileRequestContextV1(ctx, request)
	if err != nil {
		return CompileFrameRequestV1{}, mapErrorV1(op, "request", err)
	}
	if err := request.InputBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return CompileFrameRequestV1{}, withOperationV1(err, op)
	}
	digest, err := compileRequestDigestV1(request, ctx)
	if err != nil {
		return CompileFrameRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return CompileFrameRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func SealPreviewFrameRequestV1(ctx context.Context, request PreviewFrameRequestV1) (PreviewFrameRequestV1, error) {
	const op = OfflinePreviewFrameV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	if err := preflightPreviewFrameRequestV1(request); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	request, err := clonePreviewRequestContextV1(ctx, request)
	if err != nil {
		return PreviewFrameRequestV1{}, mapErrorV1(op, "request", err)
	}
	if err := request.Compiled.ContentBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return PreviewFrameRequestV1{}, withOperationV1(err, op)
	}
	digest, err := previewRequestDigestV1(request, ctx)
	if err != nil {
		return PreviewFrameRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return PreviewFrameRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func SealInspectFrameExactRequestV1(ctx context.Context, request InspectFrameExactRequestV1) (InspectFrameExactRequestV1, error) {
	const op = OfflineInspectFrameExactV1
	if err := validateSealContextAndMetaV1(ctx, request.Meta, op); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	if err := preflightInspectFrameExactRequestV1(request); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	request, err := cloneInspectRequestContextV1(ctx, request)
	if err != nil {
		return InspectFrameExactRequestV1{}, mapErrorV1(op, "request", err)
	}
	if err := request.ContentBundle.validateContextV1(ctx, request.Meta.Limits); err != nil {
		return InspectFrameExactRequestV1{}, withOperationV1(err, op)
	}
	digest, err := inspectRequestDigestV1(request, ctx)
	if err != nil {
		return InspectFrameExactRequestV1{}, mapErrorV1(op, "meta.request_digest", err)
	}
	if err := acceptOrSetRequestDigestV1(&request.Meta, digest, op); err != nil {
		return InspectFrameExactRequestV1{}, err
	}
	return request, validateContextV1(ctx, op)
}

func validateSealContextAndMetaV1(ctx context.Context, meta OfflineRequestMetaV1, op OfflineSDKOperationV1) error {
	if err := validateContextV1(ctx, op); err != nil {
		return err
	}
	if err := validateRequestMetaBaseV1(meta, op); err != nil {
		return err
	}
	if meta.RequestDigest != "" && meta.RequestDigest.Validate() != nil {
		return sdkErrorV1(OfflineErrorInvalidArgumentV1, op, "meta.request_digest", "invalid request digest", contract.ErrInvalid)
	}
	return nil
}

func acceptOrSetRequestDigestV1(meta *OfflineRequestMetaV1, digest contract.Digest, op OfflineSDKOperationV1) error {
	if meta.RequestDigest != "" && meta.RequestDigest != digest {
		return sdkErrorV1(OfflineErrorConflictV1, op, "meta.request_digest", "request digest mismatch", contract.ErrConflict)
	}
	meta.RequestDigest = digest
	return nil
}

func EncodeValidateRecipeRequestV1(ctx context.Context, request ValidateRecipeRequestV1) ([]byte, error) {
	sealed, err := SealValidateRecipeRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflineValidateRecipeV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		return 0, buffer.writeJSON(sealed)
	})
}

func EncodeCompileFrameRequestV1(ctx context.Context, request CompileFrameRequestV1) ([]byte, error) {
	sealed, err := SealCompileFrameRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflineCompileFrameV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		if err := buffer.writeLiteral(`{"meta":`); err != nil {
			return 0, err
		}
		if err := buffer.writeJSON(sealed.Meta); err != nil {
			return 0, err
		}
		for _, field := range []struct {
			name  string
			value any
		}{
			{"attempt_id", sealed.AttemptID}, {"manifest_id", sealed.ManifestID}, {"frame_id", sealed.FrameID},
			{"generation_id", sealed.GenerationID}, {"generation_ordinal", sealed.GenerationOrdinal}, {"recipe", sealed.Recipe},
			{"execution", sealed.Execution}, {"candidates", sealed.Candidates},
			{"created_unix_nano", sealed.CreatedUnixNano}, {"expires_unix_nano", sealed.ExpiresUnixNano},
		} {
			if err := writeNamedJSONFieldV1(buffer, field.name, field.value); err != nil {
				return 0, err
			}
		}
		if sealed.ParentFrame != nil {
			if err := writeNamedJSONFieldV1(buffer, "parent_frame", sealed.ParentFrame); err != nil {
				return 0, err
			}
		}
		if err := buffer.writeLiteral(`,"input_bundle":`); err != nil {
			return 0, err
		}
		contentChars, err := writeBundleJSONV1(ctx, buffer, sealed.InputBundle)
		if err != nil {
			return 0, err
		}
		return contentChars, buffer.writeLiteral(`}`)
	})
}

func EncodePreviewFrameRequestV1(ctx context.Context, request PreviewFrameRequestV1) ([]byte, error) {
	sealed, err := SealPreviewFrameRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflinePreviewFrameV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		if err := buffer.writeLiteral(`{"meta":`); err != nil {
			return 0, err
		}
		if err := buffer.writeJSON(sealed.Meta); err != nil {
			return 0, err
		}
		if err := buffer.writeLiteral(`,"compiled":`); err != nil {
			return 0, err
		}
		contentChars, err := writeCompiledJSONV1(ctx, buffer, sealed.Compiled)
		if err != nil {
			return 0, err
		}
		if err := writeNamedJSONFieldV1(buffer, "expected_compile_digest", sealed.ExpectedCompileDigest); err != nil {
			return 0, err
		}
		if err := writeNamedJSONFieldV1(buffer, "checked_unix_nano", sealed.CheckedUnixNano); err != nil {
			return 0, err
		}
		return contentChars, buffer.writeLiteral(`}`)
	})
}

func EncodeInspectFrameExactRequestV1(ctx context.Context, request InspectFrameExactRequestV1) ([]byte, error) {
	sealed, err := SealInspectFrameExactRequestV1(ctx, request)
	if err != nil {
		return nil, err
	}
	return encodeBoundedRequestV1(ctx, OfflineInspectFrameExactV1, sealed.Meta, func(buffer *boundedCodecBufferV1) (uint64, error) {
		if err := buffer.writeLiteral(`{"meta":`); err != nil {
			return 0, err
		}
		if err := buffer.writeJSON(sealed.Meta); err != nil {
			return 0, err
		}
		for _, field := range []struct {
			name  string
			value any
		}{{"manifest", sealed.Manifest}, {"frame", sealed.Frame}} {
			if err := writeNamedJSONFieldV1(buffer, field.name, field.value); err != nil {
				return 0, err
			}
		}
		if err := buffer.writeLiteral(`,"content_bundle":`); err != nil {
			return 0, err
		}
		contentChars, err := writeBundleJSONV1(ctx, buffer, sealed.ContentBundle)
		if err != nil {
			return 0, err
		}
		for _, field := range []struct {
			name  string
			value any
		}{
			{"expected_manifest_ref", sealed.ExpectedManifestRef}, {"expected_frame_ref", sealed.ExpectedFrameRef},
			{"expected_compile_digest", sealed.ExpectedCompileDigest}, {"checked_unix_nano", sealed.CheckedUnixNano},
		} {
			if err := writeNamedJSONFieldV1(buffer, field.name, field.value); err != nil {
				return 0, err
			}
		}
		return contentChars, buffer.writeLiteral(`}`)
	})
}

func encodeBoundedRequestV1(ctx context.Context, op OfflineSDKOperationV1, meta OfflineRequestMetaV1, write func(*boundedCodecBufferV1) (uint64, error)) ([]byte, error) {
	buffer := &boundedCodecBufferV1{ctx: ctx, max: meta.Limits.MaxWireRequestBytes}
	contentChars, err := write(buffer)
	if err != nil {
		return nil, mapErrorV1(op, "request", err)
	}
	payload := buffer.buf.Bytes()
	if uint64(len(payload)) > meta.Limits.MaxWireRequestBytes || uint64(len(payload)) < contentChars || uint64(len(payload))-contentChars > meta.Limits.MaxNonContentWireBytes {
		return nil, sdkErrorV1(OfflineErrorLimitExceededV1, op, "request", "request wire limit exceeded", contract.ErrLimitExceeded)
	}
	if err := validateContextV1(ctx, op); err != nil {
		return nil, err
	}
	return cloneCodecBytesV1(ctx, payload)
}

func writeNamedJSONFieldV1(buffer *boundedCodecBufferV1, name string, value any) error {
	if err := buffer.writeLiteral(`,"` + name + `":`); err != nil {
		return err
	}
	return buffer.writeJSON(value)
}

func writeBundleJSONV1(ctx context.Context, buffer *boundedCodecBufferV1, bundle OfflineContentBundleV1) (uint64, error) {
	if err := buffer.writeLiteral(`{"items":[`); err != nil {
		return 0, err
	}
	var contentChars uint64
	for itemIndex, item := range bundle.items {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if itemIndex > 0 {
			if err := buffer.writeLiteral(`,`); err != nil {
				return 0, err
			}
		}
		if err := buffer.writeLiteral(`{"ref":`); err != nil {
			return 0, err
		}
		if err := buffer.writeJSON(item.Ref); err != nil {
			return 0, err
		}
		if err := buffer.writeLiteral(`,"base64_chunks":[`); err != nil {
			return 0, err
		}
		for offset := 0; offset < len(item.Bytes); offset += rawChunkBytesV1 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			if offset > 0 {
				if err := buffer.writeLiteral(`,`); err != nil {
					return 0, err
				}
			}
			end := offset + rawChunkBytesV1
			if end > len(item.Bytes) {
				end = len(item.Bytes)
			}
			chunk := base64.StdEncoding.EncodeToString(item.Bytes[offset:end])
			if ^uint64(0)-contentChars < uint64(len(chunk)) {
				return 0, contract.ErrLimitExceeded
			}
			contentChars += uint64(len(chunk))
			if err := buffer.writeJSON(chunk); err != nil {
				return 0, err
			}
		}
		if err := buffer.writeLiteral(`]}`); err != nil {
			return 0, err
		}
	}
	if err := buffer.writeLiteral(`],"content_set_digest":`); err != nil {
		return 0, err
	}
	if err := buffer.writeJSON(bundle.ContentSetDigest()); err != nil {
		return 0, err
	}
	return contentChars, buffer.writeLiteral(`}`)
}

func writeCompiledJSONV1(ctx context.Context, buffer *boundedCodecBufferV1, compiled CompiledBundleV1) (uint64, error) {
	if err := buffer.writeLiteral(`{"manifest":`); err != nil {
		return 0, err
	}
	if err := buffer.writeJSON(compiled.Manifest); err != nil {
		return 0, err
	}
	if err := writeNamedJSONFieldV1(buffer, "frame", compiled.Frame); err != nil {
		return 0, err
	}
	if err := buffer.writeLiteral(`,"content_bundle":`); err != nil {
		return 0, err
	}
	contentChars, err := writeBundleJSONV1(ctx, buffer, compiled.ContentBundle)
	if err != nil {
		return 0, err
	}
	for _, field := range []struct {
		name  string
		value any
	}{
		{"residual_candidate_refs", compiled.ResidualCandidateRefs}, {"authoritative", compiled.Authoritative}, {"compile_digest", compiled.CompileDigest},
	} {
		if err := writeNamedJSONFieldV1(buffer, field.name, field.value); err != nil {
			return 0, err
		}
	}
	return contentChars, buffer.writeLiteral(`}`)
}
