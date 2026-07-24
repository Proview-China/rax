package sdk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestBase64PrimitiveCanonicalMatrixV1(t *testing.T) {
	lengths := []int{0, 1, 2, rawChunkBytesV1 - 1, rawChunkBytesV1, rawChunkBytesV1 + 1, 2 * rawChunkBytesV1}
	for _, length := range lengths {
		value := make([]byte, length)
		for i := range value {
			value[i] = byte(i % 251)
		}
		chunks, err := encodeBase64ChunksV1(context.Background(), value)
		if err != nil {
			t.Fatalf("length %d encode: %v", length, err)
		}
		got, err := decodeBase64ChunksV1(context.Background(), chunks)
		if err != nil {
			t.Fatalf("length %d decode: %v", length, err)
		}
		if !bytes.Equal(got, value) {
			t.Fatalf("length %d round trip mismatch", length)
		}
		if length == 0 && (chunks == nil || len(chunks) != 0) {
			t.Fatal("zero primitive must use present empty chunks")
		}
	}
	chunks, _ := encodeBase64ChunksV1(context.Background(), []byte{0xfb, 0xff})
	if !reflect.DeepEqual(chunks, []string{"+/8="}) {
		t.Fatalf("standard alphabet mismatch: %#v", chunks)
	}
	invalid := [][]string{{""}, {"-_8="}, {"AA"}, {"AAE"}, {"AA==\n"}, {"AA==", "AQ=="}}
	for _, chunks := range invalid {
		if _, err := decodeBase64ChunksV1(context.Background(), chunks); err == nil {
			t.Fatalf("accepted non-canonical chunks %#v", chunks)
		}
	}
}

func TestBase64ChunkCountAndMidCancellationV1(t *testing.T) {
	tooMany := make([]string, int((hardMaxOutputRawBytesV1+rawChunkBytesV1-1)/rawChunkBytesV1)+1)
	if _, err := decodeBase64ChunksV1(context.Background(), tooMany); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("oversized chunk vector did not fail closed: %v", err)
	}
	ctx := &countdownContextV1{Context: context.Background(), remaining: 2}
	if chunks, err := encodeBase64ChunksV1(ctx, make([]byte, 4*rawChunkBytesV1)); !errors.Is(err, context.Canceled) || chunks != nil {
		t.Fatalf("mid-stream cancellation returned partial chunks: %#v %v", chunks, err)
	}
}

func TestOfflineContentBundleRejectsZeroAndDoesNotAliasV1(t *testing.T) {
	limits := testLimitsV1(OfflineCompileFrameV1)
	if bundle, err := NewOfflineContentBundleV1([]OfflineContentItemV1{{}}, limits); err == nil || bundle.ContentSetDigest() != "" {
		t.Fatal("zero content item was accepted")
	}
	value := []byte("content")
	item := itemV1(value)
	bundle, err := NewOfflineContentBundleV1([]OfflineContentItemV1{item}, limits)
	if err != nil {
		t.Fatal(err)
	}
	value[0] = 'X'
	items := bundle.Items()
	items[0].Bytes[0] = 'Y'
	got, ok := bundle.Lookup(item.Ref)
	if !ok || string(got) != "content" {
		t.Fatalf("bundle aliased caller bytes: %q", got)
	}
}

func TestOfflineSDKEndToEndDeterministicV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	first, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("same request produced different compiled response")
	}
	if first.Compiled.Authoritative {
		t.Fatal("offline compile became authoritative")
	}
	encoded, err := EncodeCompileFrameResponseV1(context.Background(), first)
	if err != nil || len(encoded) == 0 {
		t.Fatalf("encode compile response: %v", err)
	}
	wire, err := compileResponseToWireV1(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	wantEncoded, err := json.Marshal(wire)
	if err != nil || !bytes.Equal(encoded, wantEncoded) {
		t.Fatalf("streaming codec changed canonical JSON: %v", err)
	}

	previewRequest := PreviewFrameRequestV1{
		Meta: requestMetaV1(OfflinePreviewFrameV1, "preview-1"), Compiled: first.Compiled,
		ExpectedCompileDigest: first.Compiled.CompileDigest, CheckedUnixNano: testkit.Now,
	}
	previewRequest.Meta.RequestDigest, _ = previewRequestDigestV1(previewRequest)
	preview, err := PreviewFrameV1(context.Background(), previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Fragments) != 3 || preview.TotalTokens != 38 || preview.SemiStableRef == nil {
		t.Fatalf("unexpected preview: %#v", preview)
	}

	inspectRequest := InspectFrameExactRequestV1{
		Meta: requestMetaV1(OfflineInspectFrameExactV1, "inspect-1"), Manifest: first.Compiled.Manifest,
		Frame: first.Compiled.Frame, ContentBundle: first.Compiled.ContentBundle,
		ExpectedManifestRef: preview.ManifestRef, ExpectedFrameRef: preview.FrameRef,
		ExpectedCompileDigest: first.Compiled.CompileDigest, CheckedUnixNano: testkit.Now,
	}
	inspectRequest.Meta.RequestDigest, _ = inspectRequestDigestV1(inspectRequest)
	inspection, err := InspectFrameExactV1(context.Background(), inspectRequest)
	if err != nil || !inspection.Exact {
		t.Fatalf("inspect exact: %#v %v", inspection, err)
	}
	if _, err := EncodePreviewFrameResponseV1(context.Background(), preview); err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeInspectFrameExactResponseV1(context.Background(), inspection); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredAndOptionalMissingSemanticsV1(t *testing.T) {
	required := compileFixtureRequestV1(t)
	items := required.InputBundle.Items()
	required.InputBundle, _ = NewOfflineContentBundleV1(items[1:], required.Meta.Limits)
	required.Meta.RequestDigest, _ = compileRequestDigestV1(required)
	_, err := CompileFrameV1(context.Background(), required)
	assertSDKCodeV1(t, err, OfflineErrorNotFoundV1)

	optional := compileFixtureRequestV1(t)
	items = optional.InputBundle.Items()
	filtered := make([]OfflineContentItemV1, 0, len(items)-1)
	optionalRef := optional.Candidates[1].Content
	for _, item := range items {
		if item.Ref != optionalRef {
			filtered = append(filtered, item)
		}
	}
	optional.InputBundle, _ = NewOfflineContentBundleV1(filtered, optional.Meta.Limits)
	optional.Meta.RequestDigest, _ = compileRequestDigestV1(optional)
	response, err := CompileFrameV1(context.Background(), optional)
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Compiled.ResidualCandidateRefs) != 1 || response.Compiled.ResidualCandidateRefs[0].ID != optional.Candidates[1].ID {
		t.Fatalf("optional missing did not preserve content_unavailable residual: %#v", response.Compiled.ResidualCandidateRefs)
	}
}

func TestOfflineSDKCancellationAndTamperReturnZeroV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	response, err := CompileFrameV1(ctx, request)
	if !reflect.DeepEqual(response, CompileFrameResponseV1{}) || !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel did not return zero response: %#v %v", response, err)
	}
	request.Meta.RequestDigest = testkit.D("tampered")
	response, err = CompileFrameV1(context.Background(), request)
	if !reflect.DeepEqual(response, CompileFrameResponseV1{}) {
		t.Fatal("tampered request returned partial response")
	}
	assertSDKCodeV1(t, err, OfflineErrorConflictV1)
}

func TestStrictCodecRejectsDuplicateUnknownTrailingAndZeroWireItemV1(t *testing.T) {
	meta := requestMetaV1(OfflineValidateRecipeV1, "decode-1")
	request := ValidateRecipeRequestV1{Meta: meta, Recipe: testkit.Recipe(), Candidates: []contract.ContextCandidate{}}
	request.Meta.RequestDigest, _ = canonicalDigestV1("validate-recipe-request", struct {
		Meta       OfflineRequestMetaV1        `json:"meta"`
		Recipe     contract.ContextRecipe      `json:"recipe"`
		Candidates []contract.ContextCandidate `json:"candidates"`
	}{func() OfflineRequestMetaV1 { m := request.Meta; m.RequestDigest = ""; return m }(), request.Recipe, request.Candidates})
	payload, _ := json.Marshal(request)
	if _, err := DecodeValidateRecipeRequestV1(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(payload, []byte(`"request_id":"decode-1"`), []byte(`"request_id":"decode-1","request_id":"other"`), 1)
	if _, err := DecodeValidateRecipeRequestV1(context.Background(), duplicate); err == nil {
		t.Fatal("duplicate key accepted")
	}
	unknown := bytes.Replace(payload, []byte(`"candidates":[]`), []byte(`"candidates":[],"unknown":true`), 1)
	if _, err := DecodeValidateRecipeRequestV1(context.Background(), unknown); err == nil {
		t.Fatal("unknown key accepted")
	}
	if _, err := DecodeValidateRecipeRequestV1(context.Background(), append(payload, []byte(`{}`)...)); err == nil {
		t.Fatal("trailing document accepted")
	}

	wire := offlineContentBundleWireV1{Items: []offlineContentItemWireV1{{Ref: contract.ContentRef{}, Base64Chunks: []string{}}}, ContentSetDigest: testkit.D("empty")}
	if bundle, err := decodeBundleWireV1(context.Background(), OfflineCompileFrameV1, wire, testLimitsV1(OfflineCompileFrameV1)); err == nil || bundle.ContentSetDigest() != "" {
		t.Fatal("zero wire item accepted")
	}
}

func TestStrictCodecRequiresRecursiveZeroValueFieldsV1(t *testing.T) {
	compileRequest := compileFixtureRequestV1(t)
	validateRequest := ValidateRecipeRequestV1{Meta: requestMetaV1(OfflineValidateRecipeV1, "presence-validate"), Recipe: compileRequest.Recipe, Candidates: compileRequest.Candidates}
	validateMeta := validateRequest.Meta
	validateMeta.RequestDigest = ""
	validateRequest.Meta.RequestDigest, _ = canonicalDigestV1("validate-recipe-request", struct {
		Meta       OfflineRequestMetaV1        `json:"meta"`
		Recipe     contract.ContextRecipe      `json:"recipe"`
		Candidates []contract.ContextCandidate `json:"candidates"`
	}{validateMeta, validateRequest.Recipe, validateRequest.Candidates})
	validatePayload, _ := json.Marshal(validateRequest)

	wireBundle, _ := bundleToWireV1(context.Background(), compileRequest.InputBundle)
	compileWire := compileFrameRequestWireV1{
		Meta: compileRequest.Meta, AttemptID: compileRequest.AttemptID, ManifestID: compileRequest.ManifestID, FrameID: compileRequest.FrameID,
		GenerationID: compileRequest.GenerationID, GenerationOrdinal: compileRequest.GenerationOrdinal, Recipe: compileRequest.Recipe,
		Execution: compileRequest.Execution, Candidates: compileRequest.Candidates, ParentFrame: compileRequest.ParentFrame,
		CreatedUnixNano: compileRequest.CreatedUnixNano, ExpiresUnixNano: compileRequest.ExpiresUnixNano, InputBundle: wireBundle,
	}
	compilePayload, _ := json.Marshal(compileWire)

	compiled, err := CompileFrameV1(context.Background(), compileRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewRequest := PreviewFrameRequestV1{Meta: requestMetaV1(OfflinePreviewFrameV1, "presence-preview"), Compiled: compiled.Compiled, ExpectedCompileDigest: compiled.Compiled.CompileDigest, CheckedUnixNano: testkit.Now}
	previewRequest.Meta.RequestDigest, _ = previewRequestDigestV1(previewRequest)
	compiledWire, _ := compiledToWireV1(context.Background(), compiled.Compiled)
	previewPayload, _ := json.Marshal(previewFrameRequestWireV1{Meta: previewRequest.Meta, Compiled: compiledWire, ExpectedCompileDigest: previewRequest.ExpectedCompileDigest, CheckedUnixNano: previewRequest.CheckedUnixNano})

	manifestDigest, _ := compiled.Compiled.Manifest.DigestValue()
	frameDigest, _ := compiled.Compiled.Frame.DigestValue()
	inspectRequest := InspectFrameExactRequestV1{
		Meta: requestMetaV1(OfflineInspectFrameExactV1, "presence-inspect"), Manifest: compiled.Compiled.Manifest, Frame: compiled.Compiled.Frame,
		ContentBundle:         compiled.Compiled.ContentBundle,
		ExpectedManifestRef:   contract.FactRef{ID: compiled.Compiled.Manifest.ID, Revision: compiled.Compiled.Manifest.Revision, Digest: manifestDigest},
		ExpectedFrameRef:      contract.FactRef{ID: compiled.Compiled.Frame.ID, Revision: compiled.Compiled.Frame.Revision, Digest: frameDigest},
		ExpectedCompileDigest: compiled.Compiled.CompileDigest, CheckedUnixNano: testkit.Now,
	}
	inspectRequest.Meta.RequestDigest, _ = inspectRequestDigestV1(inspectRequest)
	inspectBundleWire, _ := bundleToWireV1(context.Background(), inspectRequest.ContentBundle)
	inspectPayload, _ := json.Marshal(inspectFrameExactRequestWireV1{Meta: inspectRequest.Meta, Manifest: inspectRequest.Manifest, Frame: inspectRequest.Frame, ContentBundle: inspectBundleWire, ExpectedManifestRef: inspectRequest.ExpectedManifestRef, ExpectedFrameRef: inspectRequest.ExpectedFrameRef, ExpectedCompileDigest: inspectRequest.ExpectedCompileDigest, CheckedUnixNano: inspectRequest.CheckedUnixNano})

	tests := []struct {
		name    string
		payload []byte
		mutate  func(map[string]any)
		decode  func([]byte) error
	}{
		{"recipe", validatePayload, func(root map[string]any) { delete(objectV1(root["recipe"]), "render_version") }, func(payload []byte) error {
			_, err := DecodeValidateRecipeRequestV1(context.Background(), payload)
			return err
		}},
		{"candidate", validatePayload, func(root map[string]any) { delete(objectV1(arrayV1(root["candidates"])[0]), "required") }, func(payload []byte) error {
			_, err := DecodeValidateRecipeRequestV1(context.Background(), payload)
			return err
		}},
		{"execution", compilePayload, func(root map[string]any) { delete(objectV1(root["execution"]), "turn") }, func(payload []byte) error {
			_, err := DecodeCompileFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"content-ref", compilePayload, func(root map[string]any) {
			delete(objectV1(objectV1(arrayV1(objectV1(root["input_bundle"])["items"])[0])["ref"]), "length")
		}, func(payload []byte) error {
			_, err := DecodeCompileFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"manifest", previewPayload, func(root map[string]any) { delete(objectV1(objectV1(root["compiled"])["manifest"]), "total_tokens") }, func(payload []byte) error {
			_, err := DecodePreviewFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"decision", previewPayload, func(root map[string]any) {
			manifest := objectV1(objectV1(root["compiled"])["manifest"])
			delete(objectV1(arrayV1(manifest["decisions"])[0]), "tokens")
		}, func(payload []byte) error {
			_, err := DecodePreviewFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"fragment", previewPayload, func(root map[string]any) {
			manifest := objectV1(objectV1(root["compiled"])["manifest"])
			delete(objectV1(arrayV1(manifest["fragments"])[0]), "position")
		}, func(payload []byte) error {
			_, err := DecodePreviewFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"frame", previewPayload, func(root map[string]any) { delete(objectV1(objectV1(root["compiled"])["frame"]), "generation") }, func(payload []byte) error {
			_, err := DecodePreviewFrameRequestV1(context.Background(), payload)
			return err
		}},
		{"fact-ref", inspectPayload, func(root map[string]any) { delete(objectV1(root["expected_manifest_ref"]), "revision") }, func(payload []byte) error {
			_, err := DecodeInspectFrameExactRequestV1(context.Background(), payload)
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := mutateJSONV1(t, test.payload, test.mutate)
			assertSDKCodeV1(t, test.decode(mutated), OfflineErrorInvalidArgumentV1)
		})
	}
}

func TestCompileRequestCodecCanonicalBundleRoundTripV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	wireBundle, err := bundleToWireV1(context.Background(), request.InputBundle)
	if err != nil {
		t.Fatal(err)
	}
	wire := compileFrameRequestWireV1{
		Meta: request.Meta, AttemptID: request.AttemptID, ManifestID: request.ManifestID, FrameID: request.FrameID,
		GenerationID: request.GenerationID, GenerationOrdinal: request.GenerationOrdinal, Recipe: request.Recipe,
		Execution: request.Execution, Candidates: request.Candidates, ParentFrame: request.ParentFrame,
		CreatedUnixNano: request.CreatedUnixNano, ExpiresUnixNano: request.ExpiresUnixNano, InputBundle: wireBundle,
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCompileFrameRequestV1(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, request) {
		t.Fatalf("compile codec drifted\nwant %#v\ngot %#v", request, decoded)
	}

	mutated := wire
	mutated.InputBundle.Items = cloneSliceV1(wire.InputBundle.Items)
	mutated.InputBundle.Items[0].Base64Chunks = append([]string{"AA=="}, mutated.InputBundle.Items[0].Base64Chunks...)
	payload, _ = json.Marshal(mutated)
	if _, err := DecodeCompileFrameRequestV1(context.Background(), payload); err == nil {
		t.Fatal("redundant/short non-final chunk accepted")
	}
}

func TestEncodeRejectsTamperedResultDigestV1(t *testing.T) {
	response, err := CompileFrameV1(context.Background(), compileFixtureRequestV1(t))
	if err != nil {
		t.Fatal(err)
	}
	response.Meta.ResultDigest = testkit.D("tampered-result")
	if payload, err := EncodeCompileFrameResponseV1(context.Background(), response); err == nil || payload != nil {
		t.Fatalf("tampered result digest encoded: %v", err)
	}
}

func TestEncodeEnforcesRequestedResponseAndNestedClosureLimitsV1(t *testing.T) {
	response, err := CompileFrameV1(context.Background(), compileFixtureRequestV1(t))
	if err != nil {
		t.Fatal(err)
	}
	wireLimited := cloneCompileResponseV1(response)
	wireLimited.limits.MaxWireResponseBytes = 1
	if payload, err := EncodeCompileFrameResponseV1(context.Background(), wireLimited); err == nil || payload != nil {
		t.Fatalf("requested response wire limit was ignored: %v", err)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
	}

	validateRequest := ValidateRecipeRequestV1{Meta: requestMetaV1(OfflineValidateRecipeV1, "non-content-limit"), Recipe: compileFixtureRequestV1(t).Recipe, Candidates: []contract.ContextCandidate{}}
	meta := validateRequest.Meta
	meta.RequestDigest = ""
	validateRequest.Meta.RequestDigest, _ = canonicalDigestV1("validate-recipe-request", struct {
		Meta       OfflineRequestMetaV1        `json:"meta"`
		Recipe     contract.ContextRecipe      `json:"recipe"`
		Candidates []contract.ContextCandidate `json:"candidates"`
	}{meta, validateRequest.Recipe, validateRequest.Candidates})
	validateResponse, err := ValidateRecipeV1(context.Background(), validateRequest)
	if err != nil {
		t.Fatal(err)
	}
	validateResponse.limits.MaxNonContentWireBytes = 1
	if payload, err := EncodeValidateRecipeResponseV1(context.Background(), validateResponse); err == nil || payload != nil {
		t.Fatalf("requested non-content response limit was ignored: %v", err)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
	}

	diagnosticLimited := cloneCompileResponseV1(response)
	diagnosticLimited.limits.MaxDiagnosticMessageBytes = 1
	diagnosticLimited.Diagnostics = []OfflineDiagnosticV1{{Code: "x", Severity: "error", ObjectKind: "frame", ObjectID: "frame-1", FieldPath: "frame", Message: "too-long"}}
	if payload, err := EncodeCompileFrameResponseV1(context.Background(), diagnosticLimited); err == nil || payload != nil {
		t.Fatalf("requested diagnostic limit was ignored: %v", err)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
	}
	diagnosticCountLimited := cloneCompileResponseV1(response)
	diagnosticCountLimited.limits.MaxDiagnostics = 1
	diagnosticCountLimited.Diagnostics = []OfflineDiagnosticV1{
		{Code: "a", Severity: "error", ObjectKind: "frame", ObjectID: "frame-1", FieldPath: "frame", Message: "a"},
		{Code: "b", Severity: "warning", ObjectKind: "frame", ObjectID: "frame-1", FieldPath: "frame", Message: "b"},
	}
	if payload, err := EncodeCompileFrameResponseV1(context.Background(), diagnosticCountLimited); err == nil || payload != nil {
		t.Fatalf("requested diagnostic count limit was ignored: %v", err)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
	}

	closureTampered := cloneCompileResponseV1(response)
	closureTampered.Compiled.ResidualCandidateRefs = append(closureTampered.Compiled.ResidualCandidateRefs, response.Compiled.Manifest.Decisions[0].CandidateRef)
	if payload, err := EncodeCompileFrameResponseV1(context.Background(), closureTampered); err == nil || payload != nil {
		t.Fatalf("tampered residual closure encoded: %v", err)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorConflictV1)
	}
}

func TestCompileResponseDoesNotAliasRequestOrSiblingV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	first, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantRequestByte := request.InputBundle.items[0].Bytes[0]
	wantSiblingByte := second.Compiled.ContentBundle.items[0].Bytes[0]
	first.Compiled.ContentBundle.items[0].Bytes[0] ^= 0xff
	if request.InputBundle.items[0].Bytes[0] != wantRequestByte || second.Compiled.ContentBundle.items[0].Bytes[0] != wantSiblingByte {
		t.Fatal("compile response bytes alias request or sibling response")
	}
	first.Compiled.ResidualCandidateRefs = append(first.Compiled.ResidualCandidateRefs, contract.FactRef{})
	if len(second.Compiled.ResidualCandidateRefs) != 0 {
		t.Fatal("compile response residual slice aliases sibling response")
	}
}

func TestCompileGeneratedAndTokenLimitsUseLimitExceededV1(t *testing.T) {
	generated := compileFixtureRequestV1(t)
	generated.Meta.Limits.MaxGeneratedRawBytes = 1
	generated.Meta.RequestDigest, _ = compileRequestDigestV1(generated)
	_, err := CompileFrameV1(context.Background(), generated)
	assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)

	tokens := compileFixtureRequestV1(t)
	tokens.Meta.Limits.MaxTotalTokens = 1
	tokens.Meta.RequestDigest, _ = compileRequestDigestV1(tokens)
	_, err = CompileFrameV1(context.Background(), tokens)
	assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
}

func TestLimitBoundaryAndFaultMatrixV1(t *testing.T) {
	expectedWire := map[OfflineSDKOperationV1][2]uint64{
		OfflineValidateRecipeV1:    {hardWire48MiBV1, hardWire48MiBV1},
		OfflineCompareRecipesV1:    {hardWire48MiBV1, hardWire48MiBV1},
		OfflineCompileFrameV1:      {hardWire48MiBV1, hardWire144MiBV1},
		OfflinePreviewFrameV1:      {hardWire144MiBV1, hardWire48MiBV1},
		OfflineInspectFrameExactV1: {hardWire144MiBV1, hardWire48MiBV1},
		OfflineInspectCachePlanV1:  {hardWire48MiBV1, hardWire48MiBV1},
	}
	for _, op := range []OfflineSDKOperationV1{OfflineValidateRecipeV1, OfflineCompareRecipesV1, OfflineCompileFrameV1, OfflinePreviewFrameV1, OfflineInspectFrameExactV1, OfflineInspectCachePlanV1} {
		limits := testLimitsV1(op)
		requestCap, responseCap, ok := wireCapsV1(op)
		if !ok || [2]uint64{requestCap, responseCap} != expectedWire[op] {
			t.Fatalf("%s wire cap matrix drift: %d/%d", op, requestCap, responseCap)
		}
		if err := limits.validate(op); err != nil {
			t.Fatalf("%s exact hard limits rejected: %v", op, err)
		}
		if work := compileWorkLimitsV1(limits); work.MaxGeneratedRawBytes != 52*1024*1024 || work.MaxOutputRawBytes != 76*1024*1024 {
			t.Fatalf("%s did not clamp 68/100 MiB global watermarks to 52/76 MiB compile limits", op)
		}
		tooLarge := limits
		tooLarge.MaxInputRawBytes = hardMaxInputRawBytesV1 + 1
		if err := tooLarge.validate(op); err == nil {
			t.Fatalf("%s accepted input hard max + 1", op)
		}
		tooLarge = limits
		tooLarge.MaxGeneratedRawBytes = hardMaxGeneratedRawBytesV1 + 1
		if err := tooLarge.validate(op); err == nil {
			t.Fatalf("%s accepted generated hard max + 1", op)
		}
		tooLarge = limits
		tooLarge.MaxOutputRawBytes = hardMaxOutputRawBytesV1 + 1
		if err := tooLarge.validate(op); err == nil {
			t.Fatalf("%s accepted output hard max + 1", op)
		}
		tooLarge = limits
		tooLarge.MaxWireRequestBytes = requestCap + 1
		if err := tooLarge.validate(op); err == nil {
			t.Fatalf("%s accepted request wire max + 1", op)
		}
		tooLarge = limits
		tooLarge.MaxWireResponseBytes = responseCap + 1
		if err := tooLarge.validate(op); err == nil {
			t.Fatalf("%s accepted response wire max + 1", op)
		}
	}

	buffer := &boundedCodecBufferV1{ctx: context.Background(), max: 64}
	if err := buffer.writeBytes(make([]byte, 64)); err != nil {
		t.Fatalf("exact wire boundary rejected: %v", err)
	}
	if err := buffer.writeBytes([]byte{0}); !errors.Is(err, contract.ErrLimitExceeded) {
		t.Fatalf("wire max + 1 not classified as limit: %v", err)
	}
	if err := validateWireAccountingV1([]byte{0}, ^uint64(0), testLimitsV1(OfflineCompileFrameV1), OfflineCompileFrameV1); err == nil {
		t.Fatal("wire accounting overflow was accepted")
	} else {
		assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
	}

	request := compileFixtureRequestV1(t)
	tampered := cloneCompileRequestV1(request)
	tampered.InputBundle.items[0].Bytes[0] ^= 0xff
	tampered.Meta.RequestDigest, _ = compileRequestDigestV1(tampered)
	if response, err := CompileFrameV1(context.Background(), tampered); !reflect.DeepEqual(response, CompileFrameResponseV1{}) {
		t.Fatalf("tampered bundle returned partial response: %#v", response)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorConflictV1)
	}

	expired := compileFixtureRequestV1(t)
	expired.CreatedUnixNano = expired.Recipe.ExpiresUnixNano
	expired.ExpiresUnixNano = expired.CreatedUnixNano + 1
	expired.Meta.RequestDigest, _ = compileRequestDigestV1(expired)
	if response, err := CompileFrameV1(context.Background(), expired); !reflect.DeepEqual(response, CompileFrameResponseV1{}) {
		t.Fatalf("expired request returned partial response: %#v", response)
	} else {
		assertSDKCodeV1(t, err, OfflineErrorExpiredV1)
	}
}

func TestOfflineSDKSmallFixture64ConcurrentV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	baseline, err := CompileFrameV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, compileErr := CompileFrameV1(context.Background(), request)
			if compileErr != nil || !reflect.DeepEqual(got, baseline) {
				t.Errorf("concurrent compile mismatch: %v", compileErr)
			}
		}()
	}
	wg.Wait()
}

func compileFixtureRequestV1(t *testing.T) CompileFrameRequestV1 {
	t.Helper()
	limits := testLimitsV1(OfflineCompileFrameV1)
	values := [][]byte{[]byte("You are deterministic."), []byte("artifact-v1"), []byte("tail-v1")}
	items := make([]OfflineContentItemV1, len(values))
	for i := range values {
		items[i] = itemV1(values[i])
	}
	bundle, err := NewOfflineContentBundleV1(items, limits)
	if err != nil {
		t.Fatal(err)
	}
	request := CompileFrameRequestV1{
		Meta: requestMetaV1(OfflineCompileFrameV1, "compile-1"), AttemptID: "attempt-1", ManifestID: "manifest-1",
		FrameID: "frame-1", GenerationID: "generation-1", GenerationOrdinal: 1, Recipe: testkit.Recipe(), Execution: testkit.Execution(),
		Candidates: []contract.ContextCandidate{
			testkit.Candidate("instruction", contract.FragmentInstruction, items[0].Ref, 20),
			testkit.Candidate("artifact", contract.FragmentArtifactInline, items[1].Ref, 10),
			testkit.Candidate("conversation", contract.FragmentConversation, items[2].Ref, 8),
		},
		CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000, InputBundle: bundle,
	}
	request.Meta.RequestDigest, err = compileRequestDigestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func requestMetaV1(op OfflineSDKOperationV1, id string) OfflineRequestMetaV1 {
	return OfflineRequestMetaV1{ContractVersion: OfflineSDKContractVersionV1, RequestID: id, Operation: op, Limits: testLimitsV1(op)}
}

func testLimitsV1(op OfflineSDKOperationV1) OfflineSDKLimitsV1 {
	req, resp, _ := wireCapsV1(op)
	maxRecipes := uint32(1)
	if op == OfflineCompareRecipesV1 {
		maxRecipes = 2
	}
	return OfflineSDKLimitsV1{
		MaxRecipes: maxRecipes, MaxCandidates: 512, MaxInputContentItems: 1024, MaxInputContentItemBytes: 4 * 1024 * 1024,
		MaxInputRawBytes: 24 * 1024 * 1024, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 68 * 1024 * 1024,
		MaxOutputContentItems: 1028, MaxOutputRawBytes: 100 * 1024 * 1024, MaxTotalTokens: 1024 * 1024,
		MaxDiagnostics: 1024, MaxDiagnosticMessageBytes: 4096, MaxNonContentWireBytes: 4 * 1024 * 1024,
		MaxWireRequestBytes: req, MaxWireResponseBytes: resp,
	}
}

func itemV1(value []byte) OfflineContentItemV1 {
	digest := contract.DigestBytes(value)
	return OfflineContentItemV1{Ref: contract.ContentRef{Ref: string(digest), Digest: digest, Length: uint64(len(value))}, Bytes: append([]byte(nil), value...)}
}

func assertSDKCodeV1(t *testing.T, err error, code OfflineSDKErrorCodeV1) {
	t.Helper()
	var sdkErr *OfflineSDKErrorV1
	if !errors.As(err, &sdkErr) || sdkErr.Code != code {
		t.Fatalf("want sdk code %s, got %v", code, err)
	}
}

type countdownContextV1 struct {
	context.Context
	remaining int
}

func (c *countdownContextV1) Err() error {
	c.remaining--
	if c.remaining <= 0 {
		return context.Canceled
	}
	return nil
}

func mutateJSONV1(t *testing.T, payload []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatal(err)
	}
	mutate(root)
	result, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func objectV1(value any) map[string]any { return value.(map[string]any) }
func arrayV1(value any) []any           { return value.([]any) }
