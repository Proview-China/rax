package offlineapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/offlineapi"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestOfflineAPIValidateTypedAndJSONAgreeV1(t *testing.T) {
	request := validateRequestV1(t)
	service := offlineapi.ServiceV1{}
	typed, err := service.ValidateRecipe(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := sdk.EncodeValidateRecipeRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := service.ExecuteJSON(context.Background(), sdk.OfflineValidateRecipeV1, payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire sdk.ValidateRecipeResponseV1
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Meta != typed.Meta || wire.ReportDigest != typed.ReportDigest || wire.Valid != typed.Valid {
		t.Fatal("typed/json response drift")
	}
}

func TestOfflineAPICompareTypedAndJSONAgreeV1(t *testing.T) {
	request := compareRequestV1(t)
	service := offlineapi.ServiceV1{}
	typed, err := service.CompareRecipes(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := sdk.EncodeCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := service.ExecuteJSON(context.Background(), sdk.OfflineCompareRecipesV1, payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire sdk.CompareRecipesResponseV1
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Meta != typed.Meta || wire.Comparison.ComparisonDigest != typed.Comparison.ComparisonDigest || len(wire.Comparison.Changes) != len(typed.Comparison.Changes) {
		t.Fatal("typed/json comparison drift")
	}
}

func TestOfflineAPICacheInspectTypedAndJSONAgreeV1(t *testing.T) {
	request := cacheInspectRequestV1(t)
	service := offlineapi.ServiceV1{}
	typed, err := service.InspectCachePlan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := sdk.EncodeInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := service.ExecuteJSON(context.Background(), sdk.OfflineInspectCachePlanV1, payload)
	if err != nil {
		t.Fatal(err)
	}
	var wire sdk.InspectCachePlanResponseV1
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Meta != typed.Meta || wire.InspectionDigest != typed.InspectionDigest || wire.KeyDigest != typed.KeyDigest {
		t.Fatal("typed/json cache inspection drift")
	}
}

func TestOfflineAPICompilePreviewInspectJSONChainV1(t *testing.T) {
	service := offlineapi.ServiceV1{}
	compileRequest := compileRequestV1(t)
	compilePayload, err := sdk.EncodeCompileFrameRequestV1(context.Background(), compileRequest)
	if err != nil {
		t.Fatal(err)
	}
	compileJSON, err := service.ExecuteJSON(context.Background(), sdk.OfflineCompileFrameV1, compilePayload)
	if err != nil || !json.Valid(compileJSON) || !bytes.Contains(compileJSON, []byte(`"authoritative":false`)) {
		t.Fatalf("compile JSON failed: %v %q", err, compileJSON)
	}
	compiled, err := service.CompileFrame(context.Background(), compileRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewRequest := sdk.PreviewFrameRequestV1{
		Meta:                  requestMetaV1(sdk.OfflinePreviewFrameV1, "api-preview"),
		Compiled:              compiled.Compiled,
		ExpectedCompileDigest: compiled.Compiled.CompileDigest,
		CheckedUnixNano:       compileRequest.CreatedUnixNano,
	}
	previewPayload, err := sdk.EncodePreviewFrameRequestV1(context.Background(), previewRequest)
	if err != nil {
		t.Fatal(err)
	}
	previewJSON, err := service.ExecuteJSON(context.Background(), sdk.OfflinePreviewFrameV1, previewPayload)
	if err != nil || !json.Valid(previewJSON) {
		t.Fatalf("preview JSON failed: %v %q", err, previewJSON)
	}

	manifestDigest, err := compiled.Compiled.Manifest.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	frameDigest, err := compiled.Compiled.Frame.DigestValue()
	if err != nil {
		t.Fatal(err)
	}
	inspectRequest := sdk.InspectFrameExactRequestV1{
		Meta:                  requestMetaV1(sdk.OfflineInspectFrameExactV1, "api-inspect"),
		Manifest:              compiled.Compiled.Manifest,
		Frame:                 compiled.Compiled.Frame,
		ContentBundle:         compiled.Compiled.ContentBundle,
		ExpectedManifestRef:   contract.FactRef{ID: compiled.Compiled.Manifest.ID, Revision: compiled.Compiled.Manifest.Revision, Digest: manifestDigest},
		ExpectedFrameRef:      contract.FactRef{ID: compiled.Compiled.Frame.ID, Revision: compiled.Compiled.Frame.Revision, Digest: frameDigest},
		ExpectedCompileDigest: compiled.Compiled.CompileDigest,
		CheckedUnixNano:       compileRequest.CreatedUnixNano,
	}
	inspectPayload, err := sdk.EncodeInspectFrameExactRequestV1(context.Background(), inspectRequest)
	if err != nil {
		t.Fatal(err)
	}
	inspectJSON, err := service.ExecuteJSON(context.Background(), sdk.OfflineInspectFrameExactV1, inspectPayload)
	if err != nil || !json.Valid(inspectJSON) || !bytes.Contains(inspectJSON, []byte(`"exact":true`)) {
		t.Fatalf("inspect JSON failed: %v %q", err, inspectJSON)
	}
}

func TestOfflineAPIJSONStrictAndUnknownFailClosedV1(t *testing.T) {
	request := validateRequestV1(t)
	payload, err := sdk.EncodeValidateRecipeRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(payload, []byte(`"request_id":"api-validate"`), []byte(`"request_id":"api-validate","request_id":"duplicate"`), 1)
	if _, err := (offlineapi.ServiceV1{}).ExecuteJSON(context.Background(), sdk.OfflineValidateRecipeV1, tampered); !errors.Is(err, contract.ErrInvalid) {
		t.Fatalf("want strict duplicate-key invalid, got %v", err)
	}
	if response, err := (offlineapi.ServiceV1{}).ExecuteJSON(context.Background(), "future", payload); !errors.Is(err, contract.ErrUnsupported) || response != nil {
		t.Fatalf("want unsupported zero response, got %q %v", response, err)
	}
}

func TestOfflineAPICancelReturnsZeroResponseV1(t *testing.T) {
	request := validateRequestV1(t)
	payload, err := sdk.EncodeValidateRecipeRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if response, err := (offlineapi.ServiceV1{}).ExecuteJSON(ctx, sdk.OfflineValidateRecipeV1, payload); !errors.Is(err, context.Canceled) || response != nil {
		t.Fatalf("want canceled zero response, got %q %v", response, err)
	}
}

func validateRequestV1(t *testing.T) sdk.ValidateRecipeRequestV1 {
	t.Helper()
	requestCap, responseCap, _ := sdk.WireCapsV1(sdk.OfflineValidateRecipeV1)
	limits := sdk.OfflineSDKLimitsV1{
		MaxRecipes: 1, MaxCandidates: 8, MaxInputContentItems: 8, MaxInputContentItemBytes: 1024,
		MaxInputRawBytes: 4096, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 4096,
		MaxOutputContentItems: 12, MaxOutputRawBytes: 8192, MaxTotalTokens: 4096,
		MaxDiagnostics: 16, MaxDiagnosticMessageBytes: 512, MaxNonContentWireBytes: 1024 * 1024,
		MaxWireRequestBytes: requestCap, MaxWireResponseBytes: responseCap,
	}
	request := sdk.ValidateRecipeRequestV1{
		Meta:   sdk.OfflineRequestMetaV1{ContractVersion: sdk.OfflineSDKContractVersionV1, RequestID: "api-validate", Operation: sdk.OfflineValidateRecipeV1, Limits: limits},
		Recipe: testkit.Recipe(), Candidates: []contract.ContextCandidate{},
	}
	sealed, err := sdk.SealValidateRecipeRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func compileRequestV1(t *testing.T) sdk.CompileFrameRequestV1 {
	t.Helper()
	content := []byte("offline api instruction")
	digest := contract.DigestBytes(content)
	ref := contract.ContentRef{Ref: string(digest), Digest: digest, Length: uint64(len(content))}
	meta := requestMetaV1(sdk.OfflineCompileFrameV1, "api-compile")
	bundle, err := sdk.NewOfflineContentBundleV1([]sdk.OfflineContentItemV1{{Ref: ref, Bytes: content}}, meta.Limits)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.CompileFrameRequestV1{
		Meta: meta, AttemptID: "api-attempt", ManifestID: "api-manifest", FrameID: "api-frame",
		GenerationID: "api-generation", GenerationOrdinal: 1, Recipe: testkit.Recipe(), Execution: testkit.Execution(),
		Candidates:      []contract.ContextCandidate{testkit.Candidate("api-instruction", contract.FragmentInstruction, ref, 4)},
		CreatedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000_000, InputBundle: bundle,
	}
	sealed, err := sdk.SealCompileFrameRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func requestMetaV1(operation sdk.OfflineSDKOperationV1, requestID string) sdk.OfflineRequestMetaV1 {
	requestCap, responseCap, _ := sdk.WireCapsV1(operation)
	return sdk.OfflineRequestMetaV1{
		ContractVersion: sdk.OfflineSDKContractVersionV1, RequestID: requestID, Operation: operation,
		Limits: sdk.OfflineSDKLimitsV1{
			MaxRecipes: 1, MaxCandidates: 8, MaxInputContentItems: 8, MaxInputContentItemBytes: 1024,
			MaxInputRawBytes: 4096, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 64 * 1024,
			MaxOutputContentItems: 12, MaxOutputRawBytes: 128 * 1024, MaxTotalTokens: 4096,
			MaxDiagnostics: 16, MaxDiagnosticMessageBytes: 512, MaxNonContentWireBytes: 1024 * 1024,
			MaxWireRequestBytes: requestCap, MaxWireResponseBytes: responseCap,
		},
	}
}

func compareRequestV1(t *testing.T) sdk.CompareRecipesRequestV1 {
	t.Helper()
	meta := requestMetaV1(sdk.OfflineCompareRecipesV1, "api-compare")
	meta.Limits.MaxRecipes = 2
	base := testkit.Recipe()
	candidate := testkit.Recipe()
	candidate.SemanticVersion = "1.1.0"
	request, err := sdk.SealCompareRecipesRequestV1(context.Background(), sdk.CompareRecipesRequestV1{Meta: meta, BaseRecipe: base, CandidateRecipe: candidate, CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000_000})
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func cacheInspectRequestV1(t *testing.T) sdk.InspectCachePlanRequestV1 {
	t.Helper()
	profile := contract.ProviderCacheProfile{ContractVersion: contract.Version, ID: "api-cache-profile", Revision: 1, Provider: "provider", RouteID: "route", Model: "model", CapabilityDigest: testkit.D("cache-capability"), ExpiresUnixNano: testkit.Now + 1_000}
	profileDigest, err := profile.DigestValue(testkit.Now)
	if err != nil {
		t.Fatal(err)
	}
	partition := contract.CachePartition{
		AuditScopeDigest: testkit.D("audit"), ReuseScope: contract.ReuseRun, IsolationDigest: testkit.D("isolation"), AuthorityDigest: testkit.D("authority"), Sensitivity: contract.SensitivityInternal,
		SourceSetDigest: testkit.D("sources"), RecipeDigest: testkit.D("recipe"), RenderDigest: testkit.D("render"), ModelProfileDigest: testkit.D("model"), HarnessDigest: testkit.D("harness"), ToolSchemaDigest: testkit.D("tools"), PrefixDigest: testkit.D("prefix"),
		ProviderProfileRef: contract.FactRef{ID: profile.ID, Revision: profile.Revision, Digest: profileDigest}, KeyVersion: "v1",
	}
	request := sdk.InspectCachePlanRequestV1{
		Meta: requestMetaV1(sdk.OfflineInspectCachePlanV1, "api-cache-inspect"), CachePlan: contract.CachePlan{ContractVersion: contract.Version, ID: "api-cache-plan", Revision: 1, Partition: partition, EligibleTokens: 100, PredictedReads: 2, ReadCostPerM: 10, WriteCostPerM: 1, TTL: 200, CreatedUnixNano: testkit.Now - 100, ExpiresUnixNano: testkit.Now + 100},
		ProviderCacheProfile: profile, CheckedUnixNano: testkit.Now,
	}
	sealed, err := sdk.SealInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
