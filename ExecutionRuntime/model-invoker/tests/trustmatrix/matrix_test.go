package trustmatrix_test

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/internal/trustmatrix"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

var matrixNow = time.Date(2026, 7, 11, 9, 0, 0, 0, time.UTC)

func TestFactoryTrustMatrixMatchesLiveRegistryCatalogAndCheckedInAssets(t *testing.T) {
	matrix := buildMatrix(t)
	if len(matrix.Rows) != 18 {
		t.Fatalf("factory rows = %d, want 18", len(matrix.Rows))
	}
	csvData, err := matrix.CSV()
	if err != nil {
		t.Fatal(err)
	}
	markdownData, err := matrix.Markdown()
	if err != nil {
		t.Fatal(err)
	}
	designRoot := filepath.Join(repositoryRoot(t), ".properties.rax", "design", "model-invoker")
	assertAssetEqual(t, csvData, filepath.Join(designRoot, "factory-trust-matrix-v1candidate.csv"))
	assertAssetEqual(t, markdownData, filepath.Join(designRoot, "factory-trust-matrix-v1candidate.md"))

	var active, blocked, callableRoutes, blockedRoutes, coveredRoutes int
	for _, row := range matrix.Rows {
		factory, err := factoriesForTest(t).Get(row.AdapterID)
		if err != nil {
			t.Fatal(err)
		}
		if row.FactoryVersion == "" || row.FactoryVersion != factory.Version() {
			t.Fatalf("%s FactoryVersion = %q, live registry = %q", row.AdapterID, row.FactoryVersion, factory.Version())
		}
		callableRoutes += row.DefaultCallableRoutes
		blockedRoutes += row.HostBlockedRoutes
		if row.Scope == "default_active" {
			active++
		} else if row.Scope == "host_blocked_subscription" {
			blocked++
		}
		for _, protocol := range row.LayerA.Protocols {
			coveredRoutes += len(protocol.RouteIDs)
		}
	}
	if active != 14 || blocked != 4 || callableRoutes != 39 || blockedRoutes != 16 || coveredRoutes != 55 {
		t.Fatalf("active/blocked/callable/host-blocked/covered = %d/%d/%d/%d/%d", active, blocked, callableRoutes, blockedRoutes, coveredRoutes)
	}
}

func TestFactoryTrustMatrixProtocolAndProfileSemanticsCannotBeAggregatedAway(t *testing.T) {
	matrix := buildMatrix(t)
	rows := map[modelinvoker.ProviderID]trustmatrix.Row{}
	for _, row := range matrix.Rows {
		rows[row.AdapterID] = row
	}
	assertModes(t, rows["aws-bedrock-mantle"], map[string]string{
		"chat_completions": "pass/exact", "messages": "pass/exact", "responses": "pass/exact",
	})
	assertModes(t, rows["aws-bedrock-runtime"], map[string]string{
		"bedrock_converse": "not_applicable/indirect", "bedrock_invoke_model": "not_applicable/indirect",
	})
	assertModes(t, rows["google-vertex-ai"], map[string]string{
		"generate_content": "not_applicable/indirect", "chat_completions": "pass/exact", "messages": "pass/exact",
	})
	assertModes(t, rows["azure-openai"], map[string]string{
		"chat_completions": "not_applicable/indirect", "responses": "not_applicable/indirect",
	})
	assertModes(t, rows["gemini"], map[string]string{"generate_content": "not_applicable/indirect"})
	for adapterID, contracts := range map[modelinvoker.ProviderID]int{
		"kimi-code": 2, "minimax-token-plan": 2, "mimo-token-plan": 6, "alibaba-plan": 6,
	} {
		row := rows[adapterID]
		if len(row.LayerA.Protocols) != contracts {
			t.Fatalf("%s protocol/profile contracts = %d, want %d", adapterID, len(row.LayerA.Protocols), contracts)
		}
		for _, protocol := range row.LayerA.Protocols {
			if protocol.ResponseModel.Status != trustmatrix.StatusPass || protocol.ResponseModel.VerificationMode != "exact" || protocol.ProfileID == "" {
				t.Fatalf("%s protocol/profile contract = %#v", adapterID, protocol)
			}
		}
	}
}

func TestFactoryTrustMatrixEveryDirectAdapterUsesItsPublicConfigEndpointGate(t *testing.T) {
	const assertion = "tests/core/endpoint_trust_test.go#TestTenDirectPublicConfigsRejectRemoteAudienceAndOfficialPathEscape"
	direct := map[modelinvoker.ProviderID]struct{}{
		"openai": {}, "anthropic": {}, "gemini": {}, "deepseek": {}, "kimi": {},
		"zai": {}, "minimax": {}, "xiaomi-mimo": {}, "qwen": {}, "xai": {},
	}
	seen := map[modelinvoker.ProviderID]struct{}{}
	for _, row := range buildMatrix(t).Rows {
		_, isDirect := direct[row.AdapterID]
		usesDirectGate := len(row.LayerA.CredentialAudience.Assertions) == 1 && row.LayerA.CredentialAudience.Assertions[0] == assertion
		if isDirect != usesDirectGate {
			t.Fatalf("%s direct=%v credential endpoint assertion=%#v", row.AdapterID, isDirect, row.LayerA.CredentialAudience.Assertions)
		}
		for _, contract := range row.LayerA.Protocols {
			usesProtocolGate := len(contract.Endpoint.Assertions) == 1 && contract.Endpoint.Assertions[0] == assertion
			if isDirect != usesProtocolGate {
				t.Fatalf("%s/%s direct=%v endpoint assertion=%#v", row.AdapterID, contract.Protocol, isDirect, contract.Endpoint.Assertions)
			}
		}
		if isDirect {
			seen[row.AdapterID] = struct{}{}
		}
	}
	if len(seen) != len(direct) {
		t.Fatalf("direct Adapter coverage = %d, want exact set of %d: %#v", len(seen), len(direct), seen)
	}
}

func TestFactoryTrustMatrixEveryStatusBindsExistingCodeAndExecutableAssertion(t *testing.T) {
	matrix := buildMatrix(t)
	moduleRoot := filepath.Join(repositoryRoot(t), "ExecutionRuntime", "model-invoker")
	seenByMode := map[string]map[string]struct{}{}
	for _, row := range matrix.Rows {
		for _, check := range checks(row) {
			if check.Status == trustmatrix.StatusGap {
				t.Fatalf("unexplained gap in %s: %#v", row.AdapterID, check)
			}
			if check.Status == trustmatrix.StatusNotApplicable && (check.VerificationMode != "indirect" || strings.TrimSpace(check.Reason) == "") {
				t.Fatalf("N/A lacks indirect reason in %s: %#v", row.AdapterID, check)
			}
			for _, reference := range check.CodeEvidence {
				assertCodeReference(t, moduleRoot, reference)
			}
			for _, reference := range check.Assertions {
				assertTestReference(t, moduleRoot, reference)
				assertAssertionAllowed(t, check.VerificationMode, reference)
				if seenByMode[check.VerificationMode] == nil {
					seenByMode[check.VerificationMode] = map[string]struct{}{}
				}
				seenByMode[check.VerificationMode][reference] = struct{}{}
			}
		}
	}
	for mode, required := range requiredAssertionsByMode {
		for _, reference := range required {
			if _, ok := seenByMode[mode][reference]; !ok {
				t.Fatalf("verification mode %q lacks required executable assertion %q", mode, reference)
			}
		}
	}
}

var allowedAssertionsByMode = map[string]map[string]struct{}{
	"credential_audience": assertionSet(
		"tests/core/endpoint_trust_test.go#TestTenDirectPublicConfigsRejectRemoteAudienceAndOfficialPathEscape",
		"tests/upstream/cloud_endpoint_security_test.go#TestCloudPublicConfigsRejectUnsafeDynamicEndpointFields",
		"tests/plancompat/adapter_test.go#TestPlanConfigRejectsWrongKeyIdentityAndProductionHost",
	),
	"protocol_request_binding": assertionSet(
		"tests/protocol/binding_test.go#TestBaseRejectsSelectionAndStateMismatchBeforeNativeCall",
	),
	"catalog_exact_template": assertionSet(
		"tests/routegateway/builtin_test.go#TestEveryCallableRouteHasARealBuiltinConstructionPath",
		"tests/routegateway/builtin_test.go#TestEveryHostBlockedSubscriptionCandidateConstructsOnlyWithTrustedResolver",
	),
	"catalog_anchored_binding": assertionSet(
		"tests/routegateway/gateway_test.go#TestBindingAndSecretFailuresStopAtTheirExactBoundary",
	),
	"catalog_profile_audience": assertionSet(
		"tests/upstream/credential_test.go#TestCredentialAuthAndRouteBindings",
		"tests/upstream/route_test.go#TestRouteValidationPolicyAndCredentialBinding",
		"tests/routegateway/gateway_test.go#TestGatewayRejectsMismatchedSecretProfileAndTypedPurposeBeforeFactory",
	),
	"authoritative_gateway_stamp": assertionSet(
		"tests/routegateway/gateway_test.go#TestGatewayStampsCatalogIdentityOverCustomFactoryResponse",
		"tests/routegateway/gateway_error_test.go#TestGatewaySanitizesAndStampsEveryCapabilitiesErrorShape",
		"tests/routegateway/gateway_error_test.go#TestGatewaySecondaryModelMismatchEventIsStampedAndCloseCauseIsSafe",
	),
	"single_owner_close": assertionSet(
		"tests/routegateway/trust_closure_test.go#TestTrustedBuiltinCandidateRejectionSurvivesGatewayWithoutDoubleCloseOrLeak",
		"tests/routegateway/trust_closure_test.go#TestBuildErrorDerivesProviderCloserAndClosesOnceWithoutLeak",
		"tests/routegateway/trust_closure_test.go#TestPostBuildCallerCancellationAndDeadlineCloseLateResultOnce",
		"tests/routegateway/trust_closure_test.go#TestProviderCallFailureJoinsStaleLeaseCloseFailure",
		"tests/routegateway/gateway_test.go#TestConcurrentResolveIsSingleflightAndRotationClosesOldAdapter",
		"tests/routegateway/gateway_test.go#TestBindingVersionAndClientIdentityRotatePoolKeyAndCloseOldAdapterOnce",
		"tests/routegateway/gateway_test.go#TestStreamLeaseDefersAdapterCloseUntilStreamClose",
		"tests/routegateway/trust_closure_test.go#TestConcurrentStaleLeaseReleaseClosesOnceAndReturnsSafeFailure",
		"tests/routegateway/trust_closure_test.go#TestConcurrentGatewayCloseWaitsForRotationCloseFailure",
		"tests/routegateway/trust_closure_test.go#TestConcurrentGatewayCloseWaitsForInFlightFactoryBuildAndAggregatesCloseFailure",
		"tests/core/builtin_lifecycle_test.go#TestBuiltinCandidatePostBuildCancellationClosesConstructedProvider",
		"tests/routegateway/gateway_error_test.go#TestGatewayStreamCloseSanitizesStampsAndPreservesCloseCause",
	),
	"exact_endpoint_policy": assertionSet(
		"tests/core/endpoint_trust_test.go#TestTenDirectPublicConfigsRejectRemoteAudienceAndOfficialPathEscape",
		"tests/upstream/cloud_endpoint_security_test.go#TestCloudPublicConfigsRejectUnsafeDynamicEndpointFields",
		"tests/plancompat/adapter_test.go#TestPlanConfigRejectsWrongKeyIdentityAndProductionHost",
	),
	"adapter_owned_receipt": assertionSet(
		"tests/routegateway/builtin_test.go#TestEveryCallableRouteHasARealBuiltinConstructionPath",
		"tests/routegateway/builtin_test.go#TestEveryHostBlockedSubscriptionCandidateConstructsOnlyWithTrustedResolver",
	),
	"exact": assertionSet(
		"tests/protocol/openaichat/driver_test.go#TestDriverInvokeRejectsMissingAndMismatchedAuthoritativeModel",
		"tests/protocol/openaichat/driver_test.go#TestDriverStreamModelIdentityFailureClosesOnceWithoutPayloadOrCloseLeak",
		"tests/protocol/openairesponses/driver_test.go#TestDriverInvokeRejectsMissingAndMismatchedAuthoritativeModel",
		"tests/protocol/openairesponses/driver_test.go#TestDriverStreamModelIdentityFailureClosesOnceWithoutPayloadOrCloseLeak",
		"tests/protocol/anthropicmessages/driver_test.go#TestDriverInvokeRejectsMissingAndMismatchedAuthoritativeModel",
		"tests/protocol/anthropicmessages/driver_test.go#TestDriverStreamModelIdentityFailureClosesOnceWithoutPayloadOrCloseLeak",
		"tests/routegateway/gateway_test.go#TestGatewayRejectsProviderResponseModelDrift",
		"tests/routegateway/gateway_test.go#TestGatewayRejectsMissingProviderResponseModel",
	),
	"indirect": assertionSet(
		"tests/azureopenai/adapter_test.go#TestAzureProjectsNativeModelToDeploymentForInvokeAndStream",
		"tests/gemini/adapter_test.go#TestGenerateContentRequestResponseCapabilitiesAndRawSafety",
		"tests/vertex/adapter_test.go#TestVertexGeminiUsesProjectLocationAPIKeyAndSDKHTTPFake",
		"tests/bedrockruntime/adapter_test.go#TestConverseUsesAWSSDKSigV4AndNormalizesLocalHTTPFake",
		"tests/routegateway/gateway_test.go#TestGatewayRejectsMissingProviderResponseModel",
	),
}

var requiredAssertionsByMode = map[string][]string{
	"credential_audience":         keys(allowedAssertionsByMode["credential_audience"]),
	"protocol_request_binding":    keys(allowedAssertionsByMode["protocol_request_binding"]),
	"catalog_profile_audience":    keys(allowedAssertionsByMode["catalog_profile_audience"]),
	"authoritative_gateway_stamp": keys(allowedAssertionsByMode["authoritative_gateway_stamp"]),
	"single_owner_close":          keys(allowedAssertionsByMode["single_owner_close"]),
	"exact":                       keys(allowedAssertionsByMode["exact"]),
	"indirect":                    keys(allowedAssertionsByMode["indirect"]),
}

func assertionSet(values ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func keys(set map[string]struct{}) []string {
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	return result
}

func assertAssertionAllowed(t *testing.T, mode, reference string) {
	t.Helper()
	allowed := allowedAssertionsByMode[mode]
	if _, ok := allowed[reference]; !ok {
		t.Fatalf("verification mode %q cannot use assertion %q", mode, reference)
	}
}

func buildMatrix(t *testing.T) trustmatrix.Matrix {
	t.Helper()
	routeCatalog, err := catalog.NewDefault(matrixNow)
	if err != nil {
		t.Fatal(err)
	}
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := trustmatrix.Build(routeCatalog, factories)
	if err != nil {
		t.Fatal(err)
	}
	return matrix
}

func factoriesForTest(t *testing.T) *routegateway.FactoryRegistry {
	t.Helper()
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return factories
}

func checks(row trustmatrix.Row) []trustmatrix.Check {
	result := []trustmatrix.Check{
		row.LayerA.CredentialAudience, row.LayerA.RequestIdentity,
		row.LayerB.CatalogTemplate, row.LayerB.RuntimeBinding, row.LayerB.CredentialAudience,
		row.LayerB.GatewayIdentity, row.LayerB.Lifecycle,
	}
	for _, protocol := range row.LayerA.Protocols {
		result = append(result, protocol.Endpoint, protocol.ResponseModel)
	}
	for _, protocol := range row.LayerB.Protocols {
		result = append(result, protocol.Endpoint, protocol.ResponseModel)
	}
	return result
}

func assertModes(t *testing.T, row trustmatrix.Row, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, protocol := range row.LayerA.Protocols {
		value := string(protocol.ResponseModel.Status) + "/" + protocol.ResponseModel.VerificationMode
		if previous, exists := got[string(protocol.Protocol)]; exists && previous != value {
			t.Fatalf("%s protocol %s has conflicting modes %s/%s", row.AdapterID, protocol.Protocol, previous, value)
		}
		got[string(protocol.Protocol)] = value
	}
	if len(got) != len(want) {
		t.Fatalf("%s protocol modes = %#v, want %#v", row.AdapterID, got, want)
	}
	for protocol, value := range want {
		if got[protocol] != value {
			t.Fatalf("%s/%s mode = %q, want %q", row.AdapterID, protocol, got[protocol], value)
		}
	}
}

func assertCodeReference(t *testing.T, moduleRoot, reference string) {
	t.Helper()
	path, symbol := splitReference(t, reference)
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleRoot, filepath.FromSlash(path)), nil, 0)
	if err != nil {
		t.Fatalf("code evidence %q: %v", reference, err)
	}
	for _, declaration := range parsed.Decls {
		switch typed := declaration.(type) {
		case *ast.FuncDecl:
			candidate := typed.Name.Name
			if typed.Recv != nil && len(typed.Recv.List) == 1 {
				if receiver := receiverName(typed.Recv.List[0].Type); receiver != "" {
					candidate = receiver + "." + typed.Name.Name
				}
			}
			if candidate == symbol {
				return
			}
		case *ast.GenDecl:
			for _, spec := range typed.Specs {
				switch typedSpec := spec.(type) {
				case *ast.TypeSpec:
					if typedSpec.Name.Name == symbol {
						return
					}
				case *ast.ValueSpec:
					for _, name := range typedSpec.Names {
						if name.Name == symbol {
							return
						}
					}
				}
			}
		}
	}
	t.Fatalf("code evidence %q does not name a Go AST function, method, or type declaration", reference)
}

func receiverName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return receiverName(typed.X)
	case *ast.IndexExpr:
		return receiverName(typed.X)
	case *ast.IndexListExpr:
		return receiverName(typed.X)
	default:
		return ""
	}
}

func assertTestReference(t *testing.T, moduleRoot, reference string) {
	t.Helper()
	path, testName := splitReference(t, reference)
	if !strings.HasPrefix(path, "tests/") || !strings.HasSuffix(path, "_test.go") || !strings.HasPrefix(testName, "Test") {
		t.Fatalf("test assertion %q must name a Test function in tests/**/*_test.go", reference)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(moduleRoot, filepath.FromSlash(path)), nil, 0)
	if err != nil {
		t.Fatalf("test assertion %q: %v", reference, err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == testName && validGoTestSignature(function) {
			return
		}
	}
	t.Fatalf("test assertion %q does not name an executable test function", reference)
}

func validGoTestSignature(function *ast.FuncDecl) bool {
	if function == nil || function.Type == nil || function.Type.Params == nil || len(function.Type.Params.List) != 1 || function.Type.Results != nil {
		return false
	}
	parameter := function.Type.Params.List[0]
	if len(parameter.Names) != 1 {
		return false
	}
	pointer, ok := parameter.Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	qualified, ok := pointer.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	packageName, packageOK := qualified.X.(*ast.Ident)
	return packageOK && packageName.Name == "testing" && qualified.Sel.Name == "T"
}

func splitReference(t *testing.T, reference string) (string, string) {
	t.Helper()
	parts := strings.Split(reference, "#")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		t.Fatalf("invalid evidence reference %q", reference)
	}
	return parts[0], parts[1]
}

func assertAssetEqual(t *testing.T, generated []byte, path string) {
	t.Helper()
	checkedIn, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(generated, checkedIn) {
		t.Fatalf("factory trust matrix asset drifted: %s; run cmd/factorytrustgen", path)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
