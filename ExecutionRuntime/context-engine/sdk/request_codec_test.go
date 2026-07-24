package sdk

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
)

func TestOfflineRequestCodecRoundTripV1(t *testing.T) {
	compile := compileFixtureRequestV1(t)
	compile.Meta.RequestDigest = ""
	sealedCompile, err := SealCompileFrameRequestV1(context.Background(), compile)
	if err != nil {
		t.Fatal(err)
	}
	compilePayload, err := EncodeCompileFrameRequestV1(context.Background(), compile)
	if err != nil {
		t.Fatal(err)
	}
	decodedCompile, err := DecodeCompileFrameRequestV1(context.Background(), compilePayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sealedCompile, decodedCompile) {
		t.Fatal("compile request round-trip drift")
	}

	compileResponse, err := CompileFrameV1(context.Background(), sealedCompile)
	if err != nil {
		t.Fatal(err)
	}
	preview := PreviewFrameRequestV1{
		Meta:                  requestMetaV1(OfflinePreviewFrameV1, "preview-codec"),
		Compiled:              compileResponse.Compiled,
		ExpectedCompileDigest: compileResponse.Compiled.CompileDigest,
		CheckedUnixNano:       sealedCompile.CreatedUnixNano,
	}
	sealedPreview, err := SealPreviewFrameRequestV1(context.Background(), preview)
	if err != nil {
		t.Fatal(err)
	}
	previewPayload, err := EncodePreviewFrameRequestV1(context.Background(), preview)
	if err != nil {
		t.Fatal(err)
	}
	decodedPreview, err := DecodePreviewFrameRequestV1(context.Background(), previewPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sealedPreview, decodedPreview) {
		t.Fatal("preview request round-trip drift")
	}

	manifestDigest, err := digestJSONContextV1(context.Background(), compileResponse.Compiled.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := digestJSONContextV1(context.Background(), compileResponse.Compiled.Frame)
	if err != nil {
		t.Fatal(err)
	}
	inspect := InspectFrameExactRequestV1{
		Meta:                  requestMetaV1(OfflineInspectFrameExactV1, "inspect-codec"),
		Manifest:              compileResponse.Compiled.Manifest,
		Frame:                 compileResponse.Compiled.Frame,
		ContentBundle:         compileResponse.Compiled.ContentBundle,
		ExpectedManifestRef:   contract.FactRef{ID: compileResponse.Compiled.Manifest.ID, Revision: compileResponse.Compiled.Manifest.Revision, Digest: manifestDigest},
		ExpectedFrameRef:      contract.FactRef{ID: compileResponse.Compiled.Frame.ID, Revision: compileResponse.Compiled.Frame.Revision, Digest: frameDigest},
		ExpectedCompileDigest: compileResponse.Compiled.CompileDigest,
		CheckedUnixNano:       sealedCompile.CreatedUnixNano,
	}
	sealedInspect, err := SealInspectFrameExactRequestV1(context.Background(), inspect)
	if err != nil {
		t.Fatal(err)
	}
	inspectPayload, err := EncodeInspectFrameExactRequestV1(context.Background(), inspect)
	if err != nil {
		t.Fatal(err)
	}
	decodedInspect, err := DecodeInspectFrameExactRequestV1(context.Background(), inspectPayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sealedInspect, decodedInspect) {
		t.Fatal("inspect request round-trip drift")
	}

	validate := ValidateRecipeRequestV1{
		Meta:       requestMetaV1(OfflineValidateRecipeV1, "validate-codec"),
		Recipe:     compile.Recipe,
		Candidates: compile.Candidates,
	}
	sealedValidate, err := SealValidateRecipeRequestV1(context.Background(), validate)
	if err != nil {
		t.Fatal(err)
	}
	validatePayload, err := EncodeValidateRecipeRequestV1(context.Background(), validate)
	if err != nil {
		t.Fatal(err)
	}
	decodedValidate, err := DecodeValidateRecipeRequestV1(context.Background(), validatePayload)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sealedValidate, decodedValidate) {
		t.Fatal("validate request round-trip drift")
	}
}

func TestOfflineRequestSealRejectsDigestDriftAndCancelV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	request.Meta.RequestDigest = contract.DigestBytes([]byte("drift"))
	if _, err := SealCompileFrameRequestV1(context.Background(), request); !errors.Is(err, contract.ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}

	request.Meta.RequestDigest = ""
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if payload, err := EncodeCompileFrameRequestV1(ctx, request); !errors.Is(err, context.Canceled) || payload != nil {
		t.Fatalf("want canceled zero payload, got %d bytes %v", len(payload), err)
	}
}

func TestOfflineRequestStreamingBase64ChunksV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	value := bytes.Repeat([]byte("x"), 2*rawChunkBytesV1+1)
	item := itemV1(value)
	bundle, err := NewOfflineContentBundleV1([]OfflineContentItemV1{item}, request.Meta.Limits)
	if err != nil {
		t.Fatal(err)
	}
	request.Candidates = []contract.ContextCandidate{request.Candidates[0]}
	request.Candidates[0].Content = item.Ref
	request.InputBundle = bundle
	request.Meta.RequestDigest = ""
	payload, err := EncodeCompileFrameRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCompileFrameRequestV1(context.Background(), payload)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := decoded.InputBundle.Lookup(item.Ref)
	if !ok || !bytes.Equal(got, value) {
		t.Fatal("streamed base64 request content drift")
	}
}

func TestOfflineWireCapsPublicProjectionV1(t *testing.T) {
	for operation, want := range map[OfflineSDKOperationV1][2]uint64{
		OfflineValidateRecipeV1:    {hardWire48MiBV1, hardWire48MiBV1},
		OfflineCompareRecipesV1:    {hardWire48MiBV1, hardWire48MiBV1},
		OfflineCompileFrameV1:      {hardWire48MiBV1, hardWire144MiBV1},
		OfflinePreviewFrameV1:      {hardWire144MiBV1, hardWire48MiBV1},
		OfflineInspectFrameExactV1: {hardWire144MiBV1, hardWire48MiBV1},
		OfflineInspectCachePlanV1:  {hardWire48MiBV1, hardWire48MiBV1},
	} {
		request, response, ok := WireCapsV1(operation)
		if !ok || request != want[0] || response != want[1] {
			t.Fatalf("unexpected caps for %s: %d/%d/%v", operation, request, response, ok)
		}
	}
	if _, _, ok := WireCapsV1("future"); ok {
		t.Fatal("unknown operation must not receive caps")
	}
}
