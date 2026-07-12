// Package trustmatrix builds the versioned, machine-checkable A/B trust
// contract for every built-in Route Gateway factory. It is internal because
// these candidate assertions are implementation gates, not public SDK types.
package trustmatrix

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const Version = "praxis.model-invoker.factory-trust-matrix/v1candidate"

type Status string

const (
	StatusPass          Status = "pass"
	StatusGap           Status = "gap"
	StatusNotApplicable Status = "not_applicable"
)

type Check struct {
	Status           Status   `json:"status"`
	VerificationMode string   `json:"verification_mode"`
	Reason           string   `json:"reason,omitempty"`
	CodeEvidence     []string `json:"code_evidence"`
	Assertions       []string `json:"assertions"`
}

type ProtocolTrust struct {
	Protocol      upstream.ProtocolID          `json:"protocol"`
	ProfileID     upstream.CredentialProfileID `json:"profile_id"`
	Offering      upstream.OfferingID          `json:"offering"`
	RouteIDs      []upstream.RouteID           `json:"route_ids"`
	Endpoint      Check                        `json:"endpoint"`
	ResponseModel Check                        `json:"response_model"`
}

type LayerA struct {
	CredentialAudience Check           `json:"credential_audience"`
	RequestIdentity    Check           `json:"request_identity"`
	Protocols          []ProtocolTrust `json:"protocols"`
}

type LayerB struct {
	CatalogTemplate    Check           `json:"catalog_template"`
	RuntimeBinding     Check           `json:"runtime_binding"`
	CredentialAudience Check           `json:"credential_audience"`
	GatewayIdentity    Check           `json:"gateway_identity"`
	Lifecycle          Check           `json:"lifecycle"`
	Protocols          []ProtocolTrust `json:"protocols"`
}

type Row struct {
	FactoryID             string
	FactoryVersion        string
	AdapterID             modelinvoker.ProviderID
	Scope                 string
	DefaultCallableRoutes int
	HostBlockedRoutes     int
	LayerA                LayerA
	LayerB                LayerB
}

type Matrix struct {
	Version string
	Rows    []Row
}

type protocolRule struct {
	status     Status
	mode       string
	reason     string
	code       []string
	assertions []string
}

type routeGroup struct {
	protocol upstream.ProtocolID
	profile  upstream.CredentialProfileID
	offering upstream.OfferingID
	routes   []upstream.RouteID
}

func Build(routeCatalog *catalog.Catalog, factories *routegateway.FactoryRegistry) (Matrix, error) {
	if routeCatalog == nil || factories == nil {
		return Matrix{}, fmt.Errorf("factory trust matrix: catalog and factory registry are required")
	}
	rows := make([]Row, 0, len(factories.IDs()))
	for _, adapterID := range factories.IDs() {
		factory, err := factories.Get(adapterID)
		if err != nil {
			return Matrix{}, err
		}
		var callable, blocked int
		groups := map[string]*routeGroup{}
		for _, entry := range routeCatalog.Entries() {
			if modelinvoker.ProviderID(entry.Implementation.AdapterID) != adapterID {
				continue
			}
			included := entry.Implementation.Callable || entry.Implementation.HostActivationRequirement == catalog.HostActivationTrustedSubscriptionAuthorizationResolver
			if !included {
				continue
			}
			if entry.Implementation.Callable {
				callable++
			} else {
				blocked++
			}
			key := string(entry.Route.Protocol.ID) + "\x00" + string(entry.Route.Credential.ID) + "\x00" + string(entry.Route.Offering.ID)
			group := groups[key]
			if group == nil {
				group = &routeGroup{protocol: entry.Route.Protocol.ID, profile: entry.Route.Credential.ID, offering: entry.Route.Offering.ID}
				groups[key] = group
			}
			group.routes = append(group.routes, entry.ID)
		}
		if len(groups) == 0 {
			return Matrix{}, fmt.Errorf("factory trust matrix: AdapterID %q has no callable or host-blocked routes", adapterID)
		}
		scope := "default_active"
		if callable == 0 {
			scope = "host_blocked_subscription"
		}
		publicProtocols, gatewayProtocols, err := buildProtocolTrust(adapterID, groups)
		if err != nil {
			return Matrix{}, err
		}
		publicPath := providerPath(adapterID)
		endpointAssertion := endpointAssertion(adapterID)
		row := Row{
			FactoryID: factory.ID(), FactoryVersion: factory.Version(), AdapterID: adapterID, Scope: scope,
			DefaultCallableRoutes: callable, HostBlockedRoutes: blocked,
			LayerA: LayerA{
				CredentialAudience: pass("credential_audience", []string{publicPath + "/config.go#" + endpointPolicySymbol(adapterID)}, []string{endpointAssertion}),
				RequestIdentity: pass("protocol_request_binding", []string{
					"internal/protocol/binding.go#Binding.Validate", "internal/protocol/driver.go#Base.Validate",
				}, []string{"tests/protocol/binding_test.go#TestBaseRejectsSelectionAndStateMismatchBeforeNativeCall"}),
				Protocols: publicProtocols,
			},
			LayerB: LayerB{
				CatalogTemplate: pass("catalog_exact_template", []string{"catalog/defaults.go#DefaultDocument", "catalog/subscriptions.go#subscriptionEntry"}, []string{constructionAssertion(scope)}),
				RuntimeBinding:  pass("catalog_anchored_binding", []string{"routegateway/gateway.go#validateBinding"}, []string{"tests/routegateway/gateway_test.go#TestBindingAndSecretFailuresStopAtTheirExactBoundary"}),
				CredentialAudience: pass("catalog_profile_audience", []string{"routegateway/gateway.go#validateSecretMaterial"}, []string{
					"tests/upstream/credential_test.go#TestCredentialAuthAndRouteBindings",
					"tests/upstream/route_test.go#TestRouteValidationPolicyAndCredentialBinding",
					"tests/routegateway/gateway_test.go#TestGatewayRejectsMismatchedSecretProfileAndTypedPurposeBeforeFactory",
				}),
				GatewayIdentity: pass("authoritative_gateway_stamp", []string{"routegateway/gateway.go#stampGatewayResponse", "routegateway/gateway.go#stampGatewayError"}, []string{"tests/routegateway/gateway_test.go#TestGatewayStampsCatalogIdentityOverCustomFactoryResponse", "tests/routegateway/gateway_error_test.go#TestGatewaySanitizesAndStampsEveryCapabilitiesErrorShape", "tests/routegateway/gateway_error_test.go#TestGatewaySecondaryModelMismatchEventIsStampedAndCloseCauseIsSafe"}),
				Lifecycle: passWithReason("single_owner_close", []string{
					"internal/adaptercore/candidate_binding.go#FinalizeCandidateBinding", "routegateway/gateway.go#Gateway.Resolve",
					"routegateway/pool.go#adapterPool.acquire", "routegateway/pool.go#adapterLease.release", "routegateway/pool.go#adapterPool.close",
					"routegateway/registry.go#FactoryRegistry.Register",
				}, []string{
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
				}, "The 18 builtin factories covered by this matrix are value objects with fixed Version v1candidate. FactoryRegistry rejects replacement for an already registered AdapterID, so Factory instance hot replacement is unsupported. Gateway reads Factory.Version during every prepare and includes it in the pool key; a custom mutable Version can rotate the cached adapter, but this matrix does not claim safe mutable-Factory hot replacement. Credential, binding, and client identity versions also rotate the pool key."),
				Protocols: gatewayProtocols,
			},
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].AdapterID < rows[j].AdapterID })
	matrix := Matrix{Version: Version, Rows: rows}
	if err := matrix.Validate(); err != nil {
		return Matrix{}, err
	}
	return matrix, nil
}

func buildProtocolTrust(adapterID modelinvoker.ProviderID, groups map[string]*routeGroup) ([]ProtocolTrust, []ProtocolTrust, error) {
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	public := make([]ProtocolTrust, 0, len(keys))
	gateway := make([]ProtocolTrust, 0, len(keys))
	for _, key := range keys {
		group := groups[key]
		sort.Slice(group.routes, func(i, j int) bool { return group.routes[i] < group.routes[j] })
		rule, ok := responseRule(adapterID, group.protocol)
		if !ok {
			return nil, nil, fmt.Errorf("factory trust matrix: missing response rule for %q/%q", adapterID, group.protocol)
		}
		endpointCode := []string{providerPath(adapterID) + "/adapter.go#Adapter.CandidateBindingEndpoint"}
		endpointTests := []string{constructionAssertion(scopeFor(adapterID))}
		base := ProtocolTrust{Protocol: group.protocol, ProfileID: group.profile, Offering: group.offering, RouteIDs: append([]upstream.RouteID(nil), group.routes...)}
		a := base
		a.Endpoint = pass("exact_endpoint_policy", []string{providerPath(adapterID) + "/config.go#" + endpointPolicySymbol(adapterID), "internal/adaptercore/endpoint.go#ValidateEndpoint"}, []string{endpointAssertion(adapterID)})
		a.ResponseModel = Check{Status: rule.status, VerificationMode: rule.mode, Reason: rule.reason, CodeEvidence: append([]string(nil), rule.code...), Assertions: append([]string(nil), rule.assertions...)}
		b := base
		b.Endpoint = pass("adapter_owned_receipt", endpointCode, endpointTests)
		if rule.status == StatusNotApplicable {
			b.ResponseModel = Check{
				Status: StatusNotApplicable, VerificationMode: "indirect",
				Reason:       "Gateway can validate the portable request/deployment projection but this route exposes no authoritative upstream actual-model evidence.",
				CodeEvidence: append([]string{"routegateway/gateway.go#responseModelError"}, rule.code...),
				Assertions:   append([]string{"tests/routegateway/gateway_test.go#TestGatewayRejectsMissingProviderResponseModel"}, rule.assertions...),
			}
		} else {
			b.ResponseModel = pass("exact", append([]string{"routegateway/gateway.go#responseModelError"}, rule.code...), append([]string{
				"tests/routegateway/gateway_test.go#TestGatewayRejectsProviderResponseModelDrift",
				"tests/routegateway/gateway_test.go#TestGatewayRejectsMissingProviderResponseModel",
			}, rule.assertions...))
		}
		public = append(public, a)
		gateway = append(gateway, b)
	}
	return public, gateway, nil
}

func responseRule(adapterID modelinvoker.ProviderID, protocolID upstream.ProtocolID) (protocolRule, bool) {
	if adapterID == "azure-openai" && (protocolID == upstream.ProtocolChatCompletions || protocolID == upstream.ProtocolResponses) {
		return indirectRule(
			"Azure request identity is the configured deployment; compatibility responses may echo a different native model, so portable Model is an explicit deployment projection.",
			[]string{"provider/azureopenai/adapter.go#deploymentModelStream", "provider/azureopenai/dialect.go#dialect.VerifyResponseModel"},
			[]string{"tests/azureopenai/adapter_test.go#TestAzureProjectsNativeModelToDeploymentForInvokeAndStream"},
		), true
	}
	if protocolID == upstream.ProtocolGenerateContent {
		assertion := "tests/gemini/adapter_test.go#TestGenerateContentRequestResponseCapabilitiesAndRawSafety"
		code := "internal/protocol/geminigenerate/normalize.go#normalizeGenerateContent"
		reason := "GenerateContent portable Model is the exact request projection; upstream modelVersion is metadata and is not treated as authoritative actual-model proof."
		if adapterID == "google-vertex-ai" {
			assertion = "tests/vertex/adapter_test.go#TestVertexGeminiUsesProjectLocationAPIKeyAndSDKHTTPFake"
		}
		return indirectRule(reason, []string{code, "internal/protocol/geminigenerate/stream.go#generateContentStream.buildResponse"}, []string{assertion}), true
	}
	if protocolID == upstream.ProtocolBedrockConverse || protocolID == upstream.ProtocolBedrockInvoke {
		return indirectRule(
			"Bedrock native APIs bind the selected model in the request URI/input and do not return an authoritative actual-model field; portable Model is the request projection.",
			[]string{"internal/protocol/bedrock/mapping.go#normalizeConverseOutput", "internal/protocol/bedrock/stream.go#newConverseStream"},
			[]string{"tests/bedrockruntime/adapter_test.go#TestConverseUsesAWSSDKSigV4AndNormalizesLocalHTTPFake"},
		), true
	}
	switch protocolID {
	case upstream.ProtocolChatCompletions:
		return exactRule([]string{
			"internal/protocol/openaichat/driver.go#Driver.Invoke",
			"internal/protocol/openaichat/stream.go#stream.mapChunk",
			"internal/protocol/driver.go#Base.VerifyResponseModel",
		}, "tests/protocol/openaichat/driver_test.go"), true
	case upstream.ProtocolResponses:
		return exactRule([]string{
			"internal/protocol/openairesponses/driver.go#Driver.Invoke",
			"internal/protocol/openairesponses/stream.go#stream.mapEvent",
			"internal/protocol/driver.go#Base.VerifyResponseModel",
		}, "tests/protocol/openairesponses/driver_test.go"), true
	case upstream.ProtocolMessages:
		return exactRule([]string{
			"internal/protocol/anthropicmessages/driver.go#Driver.Invoke",
			"internal/protocol/anthropicmessages/stream.go#messageStream.mapEvent",
			"internal/protocol/driver.go#Base.VerifyResponseModel",
		}, "tests/protocol/anthropicmessages/driver_test.go"), true
	default:
		return protocolRule{}, false
	}
}

func exactRule(codePaths []string, testPath string) protocolRule {
	return protocolRule{
		status: StatusPass, mode: "exact", code: codePaths,
		assertions: []string{
			testPath + "#TestDriverInvokeRejectsMissingAndMismatchedAuthoritativeModel",
			testPath + "#TestDriverStreamModelIdentityFailureClosesOnceWithoutPayloadOrCloseLeak",
		},
	}
}

func indirectRule(reason string, code, assertions []string) protocolRule {
	return protocolRule{status: StatusNotApplicable, mode: "indirect", reason: reason, code: code, assertions: assertions}
}

func pass(mode string, code, assertions []string) Check {
	return Check{Status: StatusPass, VerificationMode: mode, CodeEvidence: code, Assertions: unique(assertions)}
}

func passWithReason(mode string, code, assertions []string, reason string) Check {
	check := pass(mode, code, assertions)
	check.Reason = reason
	return check
}

func endpointPolicySymbol(adapterID modelinvoker.ProviderID) string {
	switch adapterID {
	case "openai", "anthropic", "gemini":
		return "Config.trustedBaseURL"
	case "deepseek":
		return "Config.root"
	case "kimi", "zai":
		return "Config.endpoint"
	case "minimax", "xiaomi-mimo":
		return "Config.rootEndpoint"
	case "qwen", "xai", "aws-bedrock-runtime":
		return "Config.trustedEndpoint"
	case "aws-bedrock-mantle", "google-vertex-ai", "azure-openai":
		return "Config.trustedRootEndpoint"
	case "kimi-code", "minimax-token-plan", "mimo-token-plan", "alibaba-plan":
		return "Config.trustedEndpoint"
	default:
		return "Config"
	}
}

func providerPath(adapterID modelinvoker.ProviderID) string {
	switch adapterID {
	case "aws-bedrock-mantle":
		return "provider/bedrockmantle"
	case "aws-bedrock-runtime":
		return "provider/bedrockruntime"
	case "google-vertex-ai":
		return "provider/vertex"
	case "azure-openai":
		return "provider/azureopenai"
	case "xiaomi-mimo":
		return "provider/mimo"
	case "kimi-code", "minimax-token-plan", "mimo-token-plan", "alibaba-plan":
		return "provider/plancompat"
	default:
		return "provider/" + string(adapterID)
	}
}

func endpointAssertion(adapterID modelinvoker.ProviderID) string {
	switch adapterID {
	case "aws-bedrock-mantle", "aws-bedrock-runtime", "google-vertex-ai", "azure-openai":
		return "tests/upstream/cloud_endpoint_security_test.go#TestCloudPublicConfigsRejectUnsafeDynamicEndpointFields"
	case "kimi-code", "minimax-token-plan", "mimo-token-plan", "alibaba-plan":
		return "tests/plancompat/adapter_test.go#TestPlanConfigRejectsWrongKeyIdentityAndProductionHost"
	default:
		return "tests/core/endpoint_trust_test.go#TestTenDirectPublicConfigsRejectRemoteAudienceAndOfficialPathEscape"
	}
}

func constructionAssertion(scope string) string {
	if scope == "host_blocked_subscription" {
		return "tests/routegateway/builtin_test.go#TestEveryHostBlockedSubscriptionCandidateConstructsOnlyWithTrustedResolver"
	}
	return "tests/routegateway/builtin_test.go#TestEveryCallableRouteHasARealBuiltinConstructionPath"
}

func scopeFor(adapterID modelinvoker.ProviderID) string {
	switch adapterID {
	case "kimi-code", "minimax-token-plan", "mimo-token-plan", "alibaba-plan":
		return "host_blocked_subscription"
	default:
		return "default_active"
	}
}

func unique(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (matrix Matrix) Validate() error {
	if matrix.Version != Version {
		return fmt.Errorf("factory trust matrix: unexpected version %q", matrix.Version)
	}
	if len(matrix.Rows) != 18 {
		return fmt.Errorf("factory trust matrix: rows = %d, want 18", len(matrix.Rows))
	}
	seen := map[modelinvoker.ProviderID]struct{}{}
	active, blocked, callableRoutes, blockedRoutes := 0, 0, 0, 0
	for _, row := range matrix.Rows {
		if row.FactoryID != "builtin/"+string(row.AdapterID) || strings.TrimSpace(row.FactoryVersion) == "" || row.AdapterID == "" {
			return fmt.Errorf("factory trust matrix: invalid factory identity %q/%q", row.FactoryID, row.AdapterID)
		}
		if _, ok := seen[row.AdapterID]; ok {
			return fmt.Errorf("factory trust matrix: duplicate AdapterID %q", row.AdapterID)
		}
		seen[row.AdapterID] = struct{}{}
		callableRoutes += row.DefaultCallableRoutes
		blockedRoutes += row.HostBlockedRoutes
		if row.Scope == "default_active" && row.DefaultCallableRoutes > 0 && row.HostBlockedRoutes == 0 {
			active++
		} else if row.Scope == "host_blocked_subscription" && row.DefaultCallableRoutes == 0 && row.HostBlockedRoutes > 0 {
			blocked++
		} else {
			return fmt.Errorf("factory trust matrix: invalid scope/counts for %q", row.AdapterID)
		}
		for name, check := range map[string]Check{
			"a.credential": row.LayerA.CredentialAudience, "a.request": row.LayerA.RequestIdentity,
			"b.catalog": row.LayerB.CatalogTemplate, "b.binding": row.LayerB.RuntimeBinding,
			"b.credential": row.LayerB.CredentialAudience, "b.identity": row.LayerB.GatewayIdentity, "b.lifecycle": row.LayerB.Lifecycle,
		} {
			if err := validateCheck(row.AdapterID, name, check); err != nil {
				return err
			}
		}
		if err := validateProtocols(row); err != nil {
			return err
		}
	}
	if active != 14 || blocked != 4 || callableRoutes != 39 || blockedRoutes != 16 {
		return fmt.Errorf("factory trust matrix: active/blocked/callable/host-blocked = %d/%d/%d/%d, want 14/4/39/16", active, blocked, callableRoutes, blockedRoutes)
	}
	return nil
}

func validateProtocols(row Row) error {
	if len(row.LayerA.Protocols) == 0 || len(row.LayerA.Protocols) != len(row.LayerB.Protocols) {
		return fmt.Errorf("factory trust matrix: protocol contract count mismatch for %q", row.AdapterID)
	}
	for index := range row.LayerA.Protocols {
		a, b := row.LayerA.Protocols[index], row.LayerB.Protocols[index]
		if a.Protocol == "" || a.ProfileID == "" || a.Offering == "" || len(a.RouteIDs) == 0 ||
			a.Protocol != b.Protocol || a.ProfileID != b.ProfileID || a.Offering != b.Offering || strings.Join(routeStrings(a.RouteIDs), "\x00") != strings.Join(routeStrings(b.RouteIDs), "\x00") {
			return fmt.Errorf("factory trust matrix: protocol/profile identity mismatch for %q", row.AdapterID)
		}
		for name, check := range map[string]Check{"a.endpoint": a.Endpoint, "a.model": a.ResponseModel, "b.endpoint": b.Endpoint, "b.model": b.ResponseModel} {
			if err := validateCheck(row.AdapterID, name+"/"+string(a.Protocol)+"/"+string(a.ProfileID), check); err != nil {
				return err
			}
		}
		if a.ResponseModel.Status == StatusNotApplicable && (a.ResponseModel.VerificationMode != "indirect" || b.ResponseModel.Status != StatusNotApplicable || b.ResponseModel.VerificationMode != "indirect") {
			return fmt.Errorf("factory trust matrix: indirect A/B mismatch for %q/%q", row.AdapterID, a.Protocol)
		}
	}
	return nil
}

func validateCheck(adapterID modelinvoker.ProviderID, name string, check Check) error {
	if check.Status == StatusGap {
		return fmt.Errorf("factory trust matrix: unexplained gap for %q %s", adapterID, name)
	}
	if check.Status != StatusPass && check.Status != StatusNotApplicable {
		return fmt.Errorf("factory trust matrix: invalid status %q for %q %s", check.Status, adapterID, name)
	}
	if check.VerificationMode == "" || len(check.CodeEvidence) == 0 || len(check.Assertions) == 0 {
		return fmt.Errorf("factory trust matrix: incomplete evidence for %q %s", adapterID, name)
	}
	if check.Status == StatusNotApplicable && (check.VerificationMode != "indirect" || strings.TrimSpace(check.Reason) == "") {
		return fmt.Errorf("factory trust matrix: N/A lacks indirect reason for %q %s", adapterID, name)
	}
	for _, value := range append(append([]string(nil), check.CodeEvidence...), check.Assertions...) {
		if strings.TrimSpace(value) == "" || !strings.Contains(value, "#") {
			return fmt.Errorf("factory trust matrix: invalid evidence reference %q for %q %s", value, adapterID, name)
		}
	}
	return nil
}

func routeStrings(values []upstream.RouteID) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = string(value)
	}
	return result
}

func (matrix Matrix) CSV() ([]byte, error) {
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"matrix_version", "factory_id", "factory_version", "adapter_id", "scope", "default_callable_routes", "host_blocked_routes", "layer_a_json", "layer_b_json"}); err != nil {
		return nil, err
	}
	for _, row := range matrix.Rows {
		a, err := json.Marshal(row.LayerA)
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(row.LayerB)
		if err != nil {
			return nil, err
		}
		if err := writer.Write([]string{matrix.Version, row.FactoryID, row.FactoryVersion, string(row.AdapterID), row.Scope, fmt.Sprint(row.DefaultCallableRoutes), fmt.Sprint(row.HostBlockedRoutes), string(a), string(b)}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (matrix Matrix) Markdown() ([]byte, error) {
	if err := matrix.Validate(); err != nil {
		return nil, err
	}
	var out strings.Builder
	out.WriteString("# Factory A/B双层信任矩阵 v1candidate\n\n")
	out.WriteString("本文件由 `cmd/factorytrustgen` 从 live Catalog、Builtin Registry 与 `internal/trustmatrix` 合同生成；CSV保持严格18个Factory数据行，本文展开protocol/profile子合同。\n\n")
	out.WriteString("| Factory | FactoryVersion | Scope | callable | host-blocked | protocol/profile子合同 |\n|---|---|---:|---:|---:|---:|\n")
	for _, row := range matrix.Rows {
		fmt.Fprintf(&out, "| `%s` | `%s` | `%s` | %d | %d | %d |\n", row.FactoryID, row.FactoryVersion, row.Scope, row.DefaultCallableRoutes, row.HostBlockedRoutes, len(row.LayerA.Protocols))
	}
	for _, row := range matrix.Rows {
		fmt.Fprintf(&out, "\n## `%s`\n\n", row.FactoryID)
		out.WriteString("| Protocol | Profile | Routes | A Endpoint | A Response Model | B Receipt | B Response Model |\n|---|---|---:|---|---|---|---|\n")
		for index := range row.LayerA.Protocols {
			a, b := row.LayerA.Protocols[index], row.LayerB.Protocols[index]
			fmt.Fprintf(&out, "| `%s` | `%s` | %d | `%s/%s` | `%s/%s` | `%s/%s` | `%s/%s` |\n",
				a.Protocol, a.ProfileID, len(a.RouteIDs), a.Endpoint.Status, a.Endpoint.VerificationMode,
				a.ResponseModel.Status, a.ResponseModel.VerificationMode, b.Endpoint.Status, b.Endpoint.VerificationMode,
				b.ResponseModel.Status, b.ResponseModel.VerificationMode)
		}
		var indirectReasons []string
		for _, contract := range row.LayerA.Protocols {
			if contract.ResponseModel.Status == StatusNotApplicable {
				indirectReasons = append(indirectReasons, fmt.Sprintf("- `%s/%s` indirect理由：%s", contract.Protocol, contract.ProfileID, contract.ResponseModel.Reason))
			}
		}
		if len(indirectReasons) > 0 {
			out.WriteString("\n")
			out.WriteString(strings.Join(indirectReasons, "\n"))
			out.WriteString("\n")
		}
	}
	return []byte(out.String()), nil
}
