package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestTypedPreflightRunsBeforeCloneAndDoesNotAllocateOnSuccessV1(t *testing.T) {
	request := compileFixtureRequestV1(t)
	if allocations := testing.AllocsPerRun(1000, func() {
		if err := preflightCompileFrameRequestV1(request); err != nil {
			t.Fatal(err)
		}
	}); allocations != 0 {
		t.Fatalf("typed preflight allocated: %.2f", allocations)
	}

	over := request
	over.Meta.Limits.MaxCandidates = 1
	over.Candidates = make([]contract.ContextCandidate, 2, 1<<20)
	response, err := CompileFrameV1(context.Background(), over)
	if !reflect.DeepEqual(response, CompileFrameResponseV1{}) {
		t.Fatalf("preflight failure returned partial response: %#v", response)
	}
	assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
}

func TestWirePreflightRejectsChunkVectorBeforeDTOArrayDecodeV1(t *testing.T) {
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
	fullChunk := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0xa5}, rawChunkBytesV1))
	count := int((request.Meta.Limits.MaxInputContentItemBytes+rawChunkBytesV1-1)/rawChunkBytesV1) + 1
	wire.InputBundle.Items[0].Base64Chunks = make([]string, count)
	for index := range wire.InputBundle.Items[0].Base64Chunks {
		wire.InputBundle.Items[0].Base64Chunks[index] = fullChunk
	}
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeCompileFrameRequestV1(context.Background(), payload)
	if !reflect.DeepEqual(decoded, CompileFrameRequestV1{}) {
		t.Fatalf("chunk-vector failure returned partial request: %#v", decoded)
	}
	assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)
}

func TestWireRequestLimitAndCancellationArePreservedBeforeDecodeV1(t *testing.T) {
	request := ValidateRecipeRequestV1{Meta: requestMetaV1(OfflineValidateRecipeV1, "wire-limit"), Recipe: testkit.Recipe(), Candidates: []contract.ContextCandidate{}}
	request.Meta.Limits.MaxWireRequestBytes = 1
	request.Meta.RequestDigest = testkit.D("wire-limit-request")
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = DecodeValidateRecipeRequestV1(context.Background(), payload)
	assertSDKCodeV1(t, err, OfflineErrorLimitExceededV1)

	large := append([]byte(`{"padding":"`), bytes.Repeat([]byte("x"), 4*wireChunkBytesV1)...)
	large = append(large, []byte(`"}`)...)
	ctx := &countdownContextV1{Context: context.Background(), remaining: 3}
	if _, err := DecodeValidateRecipeRequestV1(ctx, large); !errors.Is(err, context.Canceled) {
		t.Fatalf("wire preflight remapped cancellation: %v", err)
	}

	digestCtx := &countdownContextV1{Context: context.Background(), remaining: 3}
	if _, err := canonicalDigestV1("cancel-check", make([]uint64, 4096), digestCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canonical digest remapped cancellation: %v", err)
	}
	requestDigestCtx := &countdownContextV1{Context: context.Background(), remaining: 3}
	if err := validateCompileRequestDigestV1(compileFixtureRequestV1(t), requestDigestCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("request digest validation remapped cancellation: %v", err)
	}
}

func TestStrictPresenceRejectsNullArraysAndConditionalDecisionRegionV1(t *testing.T) {
	compileRequest := compileFixtureRequestV1(t)
	compiled, err := CompileFrameV1(context.Background(), compileRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewRequest := PreviewFrameRequestV1{Meta: requestMetaV1(OfflinePreviewFrameV1, "strict-presence"), Compiled: compiled.Compiled, ExpectedCompileDigest: compiled.Compiled.CompileDigest, CheckedUnixNano: testkit.Now}
	previewRequest.Meta.RequestDigest, _ = previewRequestDigestV1(previewRequest)
	compiledWire, err := compiledToWireV1(context.Background(), compiled.Compiled)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(previewFrameRequestWireV1{Meta: previewRequest.Meta, Compiled: compiledWire, ExpectedCompileDigest: previewRequest.ExpectedCompileDigest, CheckedUnixNano: previewRequest.CheckedUnixNano})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"decisions-null", func(root map[string]any) { objectV1(objectV1(root["compiled"])["manifest"])["decisions"] = nil }},
		{"fragments-null", func(root map[string]any) { objectV1(objectV1(root["compiled"])["manifest"])["fragments"] = nil }},
		{"admitted-region-absent", func(root map[string]any) {
			manifest := objectV1(objectV1(root["compiled"])["manifest"])
			delete(objectV1(arrayV1(manifest["decisions"])[0]), "region")
		}},
		{"non-admitted-region-present", func(root map[string]any) {
			manifest := objectV1(objectV1(root["compiled"])["manifest"])
			objectV1(arrayV1(manifest["decisions"])[0])["disposition"] = string(contract.AdmissionExcluded)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mutated := mutateJSONV1(t, payload, test.mutate)
			_, err := DecodePreviewFrameRequestV1(context.Background(), mutated)
			assertSDKCodeV1(t, err, OfflineErrorInvalidArgumentV1)
		})
	}

	validate := ValidateRecipeRequestV1{Meta: requestMetaV1(OfflineValidateRecipeV1, "rules-null"), Recipe: compileRequest.Recipe, Candidates: []contract.ContextCandidate{}}
	validatePayload, _ := json.Marshal(validate)
	validatePayload = mutateJSONV1(t, validatePayload, func(root map[string]any) { objectV1(root["recipe"])["rules"] = nil })
	_, err = DecodeValidateRecipeRequestV1(context.Background(), validatePayload)
	assertSDKCodeV1(t, err, OfflineErrorInvalidArgumentV1)
}
