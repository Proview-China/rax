package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/sdk"
)

func TestCLIValidateSuccessV1(t *testing.T) {
	payload := cliValidatePayloadV1(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"recipe", "validate"}, bytes.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 || !json.Valid(bytes.TrimSpace(stdout.Bytes())) {
		t.Fatalf("unexpected streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLICompareSuccessV1(t *testing.T) {
	payload := cliComparePayloadV1(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"recipe", "compare"}, bytes.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 || !json.Valid(bytes.TrimSpace(stdout.Bytes())) || !bytes.Contains(stdout.Bytes(), []byte(`"comparison"`)) {
		t.Fatalf("unexpected streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLICacheInspectSuccessV1(t *testing.T) {
	payload := cliCacheInspectPayloadV1(t)
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"cache", "inspect"}, bytes.NewReader(payload), &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stderr.Len() != 0 || !json.Valid(bytes.TrimSpace(stdout.Bytes())) || !bytes.Contains(stdout.Bytes(), []byte(`"economic_decision"`)) || bytes.Contains(stdout.Bytes(), []byte(`"cache_hit"`)) {
		t.Fatalf("unexpected streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCLICommandAndTypedExitMatrixV1(t *testing.T) {
	for _, test := range []struct {
		args []string
	}{
		{[]string{"recipe", "compile"}}, {[]string{"recipe", "preview"}}, {[]string{"frame", "inspect"}},
	} {
		var stdout, stderr bytes.Buffer
		if code := run(context.Background(), test.args, strings.NewReader(`{}`), &stdout, &stderr); code != 2 {
			t.Fatalf("args=%v exit=%d stderr=%s", test.args, code, stderr.String())
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"invalid_argument"`) {
			t.Fatalf("args=%v unexpected streams stdout=%q stderr=%q", test.args, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"recipe", "publish"}, strings.NewReader("secret-value"), &stdout, &stderr); code != 2 {
		t.Fatalf("unknown command exit=%d", code)
	}
	if strings.Contains(stderr.String(), "secret-value") || stdout.Len() != 0 {
		t.Fatal("usage error leaked request body")
	}
}

func TestCLICanceledDoesNotEmitSuccessV1(t *testing.T) {
	payload := cliValidatePayloadV1(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := run(ctx, []string{"recipe", "validate"}, bytes.NewReader(payload), &stdout, &stderr); code != 5 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), `"code":"canceled"`) {
		t.Fatalf("unexpected streams stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func cliValidatePayloadV1(t *testing.T) []byte {
	t.Helper()
	requestCap, responseCap, _ := sdk.WireCapsV1(sdk.OfflineValidateRecipeV1)
	request := sdk.ValidateRecipeRequestV1{
		Meta: sdk.OfflineRequestMetaV1{
			ContractVersion: sdk.OfflineSDKContractVersionV1, RequestID: "cli-validate", Operation: sdk.OfflineValidateRecipeV1,
			Limits: sdk.OfflineSDKLimitsV1{
				MaxRecipes: 1, MaxCandidates: 8, MaxInputContentItems: 8, MaxInputContentItemBytes: 1024,
				MaxInputRawBytes: 4096, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 4096,
				MaxOutputContentItems: 12, MaxOutputRawBytes: 8192, MaxTotalTokens: 4096,
				MaxDiagnostics: 16, MaxDiagnosticMessageBytes: 512, MaxNonContentWireBytes: 1024 * 1024,
				MaxWireRequestBytes: requestCap, MaxWireResponseBytes: responseCap,
			},
		},
		Recipe: testkit.Recipe(), Candidates: []contract.ContextCandidate{},
	}
	payload, err := sdk.EncodeValidateRecipeRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func cliComparePayloadV1(t *testing.T) []byte {
	t.Helper()
	requestCap, responseCap, _ := sdk.WireCapsV1(sdk.OfflineCompareRecipesV1)
	base := testkit.Recipe()
	candidate := testkit.Recipe()
	candidate.SemanticVersion = "1.1.0"
	request := sdk.CompareRecipesRequestV1{
		Meta: sdk.OfflineRequestMetaV1{
			ContractVersion: sdk.OfflineSDKContractVersionV1, RequestID: "cli-compare", Operation: sdk.OfflineCompareRecipesV1,
			Limits: sdk.OfflineSDKLimitsV1{
				MaxRecipes: 2, MaxCandidates: 8, MaxInputContentItems: 8, MaxInputContentItemBytes: 1024,
				MaxInputRawBytes: 4096, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 4096,
				MaxOutputContentItems: 12, MaxOutputRawBytes: 8192, MaxTotalTokens: 4096,
				MaxDiagnostics: 16, MaxDiagnosticMessageBytes: 512, MaxNonContentWireBytes: 1024 * 1024,
				MaxWireRequestBytes: requestCap, MaxWireResponseBytes: responseCap,
			},
		},
		BaseRecipe: base, CandidateRecipe: candidate, CheckedUnixNano: testkit.Now, ExpiresUnixNano: testkit.Now + 1_000_000,
	}
	payload, err := sdk.EncodeCompareRecipesRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func cliCacheInspectPayloadV1(t *testing.T) []byte {
	t.Helper()
	requestCap, responseCap, _ := sdk.WireCapsV1(sdk.OfflineInspectCachePlanV1)
	profile := contract.ProviderCacheProfile{ContractVersion: contract.Version, ID: "cli-cache-profile", Revision: 1, Provider: "provider", RouteID: "route", Model: "model", CapabilityDigest: testkit.D("cache-capability"), ExpiresUnixNano: testkit.Now + 1_000}
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
		Meta: sdk.OfflineRequestMetaV1{ContractVersion: sdk.OfflineSDKContractVersionV1, RequestID: "cli-cache-inspect", Operation: sdk.OfflineInspectCachePlanV1, Limits: sdk.OfflineSDKLimitsV1{
			MaxRecipes: 1, MaxCandidates: 8, MaxInputContentItems: 8, MaxInputContentItemBytes: 1024, MaxInputRawBytes: 4096, MaxGeneratedContentItems: 4, MaxGeneratedRawBytes: 4096,
			MaxOutputContentItems: 12, MaxOutputRawBytes: 8192, MaxTotalTokens: 4096, MaxDiagnostics: 16, MaxDiagnosticMessageBytes: 512, MaxNonContentWireBytes: 1024 * 1024,
			MaxWireRequestBytes: requestCap, MaxWireResponseBytes: responseCap,
		}},
		CachePlan:            contract.CachePlan{ContractVersion: contract.Version, ID: "cli-cache-plan", Revision: 1, Partition: partition, EligibleTokens: 100, PredictedReads: 2, ReadCostPerM: 10, WriteCostPerM: 1, TTL: 200, CreatedUnixNano: testkit.Now - 100, ExpiresUnixNano: testkit.Now + 100},
		ProviderCacheProfile: profile, CheckedUnixNano: testkit.Now,
	}
	payload, err := sdk.EncodeInspectCachePlanRequestV1(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
