package routefacade_test

import (
	"context"
	"errors"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var testNow = time.Date(2026, 7, 18, 2, 30, 0, 0, time.UTC)

func TestRouteFacadeV1BindsCatalogOwnedSelectorsWithoutMutatingCall(t *testing.T) {
	provider := &fakeProvider{id: "openai"}
	routed := newDefaultRouteInvoker(t, provider)
	call := modelinvoker.RouteCall{
		RouteID:    "openai.direct.payg.responses",
		Invocation: generalInvocation(),
		Request: modelinvoker.Request{
			Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")},
		},
	}

	selection, err := routed.Resolve(call)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if provider.capabilityCalls != 0 || provider.invokeCalls != 0 || provider.streamCalls != 0 {
		t.Fatalf("Resolve() contacted provider: capabilities=%d invoke=%d stream=%d", provider.capabilityCalls, provider.invokeCalls, provider.streamCalls)
	}
	if selection.RouteID != call.RouteID || selection.AdapterID != "openai" || selection.Protocol != modelinvoker.ProtocolResponses || selection.Endpoint != "https://api.openai.com/v1" || !selection.Policy.Allowed {
		t.Fatalf("Resolve() selection = %#v", selection)
	}

	result, err := routed.Invoke(context.Background(), call)
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if provider.capabilityCalls != 1 || provider.invokeCalls != 1 {
		t.Fatalf("provider calls = capabilities %d, invoke %d", provider.capabilityCalls, provider.invokeCalls)
	}
	if got := provider.lastRequest; got.Provider != "openai" || got.Protocol != modelinvoker.ProtocolResponses || got.Endpoint != "https://api.openai.com/v1" || got.Model != "gpt-5.5" {
		t.Fatalf("bound request = %#v", got)
	}
	if call.Request.Provider != "" || call.Request.Protocol != modelinvoker.ProtocolAuto || call.Request.Endpoint != "" {
		t.Fatalf("caller request was mutated: %#v", call.Request)
	}
	if result.Response.Status != modelinvoker.ResponseStatusCompleted || result.Route.RouteID != call.RouteID || result.Route.EvidenceDigest == "" {
		t.Fatalf("routed result = %#v", result)
	}
}

func TestRouteFacadeRejectsCallerSelectorsAndStaticCatalogModelBeforeProvider(t *testing.T) {
	provider := &fakeProvider{id: "xai"}
	routed := newDefaultRouteInvoker(t, provider)
	base := modelinvoker.RouteCall{
		RouteID: "xai.api.global.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "grok-4.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}

	selected := base
	selected.Request.Provider = "xai"
	assertInvokeErrorCode(t, routed, selected, "route_selector_owned")

	wrongModel := base
	wrongModel.Request.Model = "grok-other"
	assertInvokeErrorCode(t, routed, wrongModel, "route_model_rejected")
	if provider.capabilityCalls != 0 || provider.invokeCalls != 0 {
		t.Fatalf("rejected calls contacted provider: capabilities=%d invoke=%d", provider.capabilityCalls, provider.invokeCalls)
	}
}

func TestRouteFacadeRejectsNonCallableSubscriptionControlRecordBeforeProvider(t *testing.T) {
	provider := &fakeProvider{id: "openai"}
	routeCatalog, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatal(err)
	}
	var routeID upstream.RouteID
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable && (entry.Route.Offering.Kind == upstream.OfferingTokenPlan || entry.Route.Offering.Kind == upstream.OfferingCodingPlan) {
			routeID = entry.ID
			break
		}
	}
	if routeID == "" {
		t.Fatal("default catalog has no subscription control record")
	}
	routed := newRouteInvoker(t, routeCatalog, provider)
	call := modelinvoker.RouteCall{
		RouteID: routeID, Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "unused", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	assertInvokeErrorCode(t, routed, call, "route_not_callable")
	if provider.capabilityCalls != 0 || provider.invokeCalls != 0 || provider.streamCalls != 0 {
		t.Fatalf("control record contacted provider: %#v", provider)
	}
}

func TestRouteFacadeRequiresHostResolvedSubscriptionAuthorization(t *testing.T) {
	provider := &fakeProvider{id: "openai"}
	routeCatalog, route := callableSubscriptionCatalog(t)
	trustedInvocation := upstream.InvocationContext{
		Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal,
		Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground,
		ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest},
	}
	quota := int64(10)
	trustedEntitlement := upstream.EntitlementState{
		OfferingID: route.Offering.ID, CredentialProfile: route.Credential.ID, Status: upstream.EntitlementActive,
		ObservedAt: testNow.Add(-time.Minute), ValidUntil: testNow.Add(time.Minute), RemainingQuota: &quota,
	}
	call := modelinvoker.RouteCall{
		RouteID: route.ID,
		Request: modelinvoker.Request{Model: "subscription-model", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}

	withoutResolver := newRouteInvoker(t, routeCatalog, provider)
	assertInvokeErrorCode(t, withoutResolver, call, "subscription_authorization_resolver_required")

	resolver := &trustedAuthorizationResolver{authorization: modelinvoker.SubscriptionAuthorization{Invocation: trustedInvocation, Entitlement: trustedEntitlement}}
	routed := newRouteInvoker(t, routeCatalog, provider, modelinvoker.WithSubscriptionAuthorizationResolver(resolver))
	for _, forged := range []modelinvoker.RouteCall{
		func() modelinvoker.RouteCall { value := call; value.Invocation = trustedInvocation; return value }(),
		func() modelinvoker.RouteCall {
			value := call
			value.Invocation = trustedInvocation
			value.Invocation.ClientIdentity.Source = upstream.ClientIdentityRuntimeObserved
			return value
		}(),
		func() modelinvoker.RouteCall {
			value := call
			value.EntitlementState = &trustedEntitlement
			return value
		}(),
	} {
		assertInvokeErrorCode(t, routed, forged, "untrusted_subscription_claims")
	}
	if resolver.calls != 0 {
		t.Fatalf("forged claims reached trusted resolver %d times", resolver.calls)
	}
	if provider.capabilityCalls != 0 || provider.invokeCalls != 0 {
		t.Fatalf("forged claims contacted provider: capabilities=%d invoke=%d", provider.capabilityCalls, provider.invokeCalls)
	}

	result, err := routed.Invoke(context.Background(), call)
	if err != nil {
		t.Fatalf("authorized subscription Invoke() error = %v", err)
	}
	if !result.Route.Policy.Allowed || result.Route.Policy.AllowsAutomaticPAYGSwitch || provider.invokeCalls != 1 {
		t.Fatalf("authorized subscription result=%#v provider=%#v", result.Route, provider)
	}

	if resolver.calls != 1 {
		t.Fatalf("trusted resolver calls = %d, want 1", resolver.calls)
	}
}

func TestRouteFacadeStreamCarriesSelectionAndWrapsTerminalErrors(t *testing.T) {
	provider := &fakeProvider{id: "openai", stream: &fakeStream{events: []modelinvoker.StreamEvent{{Type: modelinvoker.StreamEventTextDelta, TextDelta: "ok"}}}}
	routed := newDefaultRouteInvoker(t, provider)
	call := modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	stream, err := routed.Stream(context.Background(), call)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	if stream.Route().RouteID != call.RouteID || !stream.Next() || stream.Event().TextDelta != "ok" || stream.Next() || stream.Err() != nil {
		t.Fatalf("routed stream state route=%#v event=%#v err=%v", stream.Route(), stream.Event(), stream.Err())
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestRouteFacadeStreamWrapsTerminalProviderErrorWithRouteSelection(t *testing.T) {
	provider := &fakeProvider{id: "openai", stream: &fakeStream{terminalErr: errors.New("transport ended")}}
	routed := newDefaultRouteInvoker(t, provider)
	call := modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	stream, err := routed.Stream(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if stream.Next() {
		t.Fatal("Next() = true for terminal failure")
	}
	var routeError *modelinvoker.RouteError
	var invocationError *modelinvoker.Error
	if !errors.As(stream.Err(), &routeError) || routeError.Route.RouteID != call.RouteID || !errors.As(stream.Err(), &invocationError) || invocationError.Provider != "openai" {
		t.Fatalf("terminal error lost route/provider identity: route=%#v invocation=%#v", routeError, invocationError)
	}
}

func TestRouteFacadeConstructorAndUnknownRouteFailWithoutProviderCalls(t *testing.T) {
	if _, err := modelinvoker.NewRouteInvoker(nil, nil); err == nil {
		t.Fatal("NewRouteInvoker(nil, nil) error = nil")
	}
	provider := &fakeProvider{id: "openai"}
	routed := newDefaultRouteInvoker(t, provider)
	call := modelinvoker.RouteCall{
		RouteID: "missing.route", Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	assertInvokeErrorCode(t, routed, call, "route_not_found")
	if provider.capabilityCalls != 0 || provider.invokeCalls != 0 || provider.streamCalls != 0 {
		t.Fatalf("unknown route contacted provider: %#v", provider)
	}
}

func TestRouteFacadeRejectsEvidenceThatExpiresAfterCatalogConstruction(t *testing.T) {
	provider := &fakeProvider{id: "openai"}
	routeCatalog, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := modelinvoker.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := routeCatalog.Get("openai.direct.payg.responses")
	if !ok {
		t.Fatal("OpenAI Responses route missing")
	}
	routed, err := modelinvoker.NewRouteInvoker(routeCatalog, invoker, modelinvoker.WithRouteClock(func() time.Time {
		return entry.Evidence.ValidUntil
	}))
	if err != nil {
		t.Fatal(err)
	}
	call := modelinvoker.RouteCall{
		RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	assertInvokeErrorCode(t, routed, call, "route_evidence_unavailable")
}

func TestV1CandidateIdentifiersAndCompatibilityAliases(t *testing.T) {
	if modelinvoker.SemanticPrimitivesCandidateVersion != "praxis.model-invoker.semantic/v1candidate" ||
		modelinvoker.RoutePolicyCandidateVersion != "praxis.model-invoker.route-policy/v1candidate" {
		t.Fatalf("unexpected candidate identifiers: %q %q", modelinvoker.SemanticPrimitivesCandidateVersion, modelinvoker.RoutePolicyCandidateVersion)
	}
	if modelinvoker.SemanticPrimitivesVersion != modelinvoker.SemanticPrimitivesCandidateVersion ||
		modelinvoker.RouteFacadeVersion != modelinvoker.RoutePolicyCandidateVersion {
		t.Fatal("candidate compatibility aliases drifted")
	}
}

func newDefaultRouteInvoker(t *testing.T, provider *fakeProvider) *modelinvoker.RouteInvoker {
	t.Helper()
	routeCatalog, err := catalog.NewDefault(testNow)
	if err != nil {
		t.Fatal(err)
	}
	return newRouteInvoker(t, routeCatalog, provider)
}

func newRouteInvoker(t *testing.T, routeCatalog *catalog.Catalog, provider *fakeProvider, options ...modelinvoker.RouteInvokerOption) *modelinvoker.RouteInvoker {
	t.Helper()
	registry, err := modelinvoker.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	invoker, err := modelinvoker.NewInvoker(registry)
	if err != nil {
		t.Fatal(err)
	}
	options = append([]modelinvoker.RouteInvokerOption{modelinvoker.WithRouteClock(func() time.Time { return testNow })}, options...)
	routed, err := modelinvoker.NewRouteInvoker(routeCatalog, invoker, options...)
	if err != nil {
		t.Fatal(err)
	}
	return routed
}

func callableSubscriptionCatalog(t *testing.T) (*catalog.Catalog, upstream.UpstreamRoute) {
	t.Helper()
	document := catalog.DefaultDocument()
	entry := document.Entries[0].Clone()
	entry.ID = "test.subscription.interactive.responses"
	entry.Route.ID = entry.ID
	entry.Route.Offering = upstream.Offering{
		ID: "test.interactive-plan", Kind: upstream.OfferingTokenPlan,
		Entitlement: upstream.CommercialEntitlement{
			AllowedUsage: upstream.AllowedUsageInteractiveCodingOnly, RequiresExplicitContext: true,
			SubjectPolicy: upstream.SubjectPolicyPersonalOnly, TenancyPolicy: upstream.TenancyPolicySingleTenantOnly,
			ExecutionPolicy: upstream.ExecutionPolicyForegroundOnly, ProductionPolicy: upstream.ProductionPolicyForbidden,
			RequiresClientIdentity: true, AllowedClientNames: []string{"praxis-cli"},
		},
	}
	entry.Route.Credential.AllowedOfferingIDs = []upstream.OfferingID{entry.Route.Offering.ID}
	entry.Route.Model.ProviderModelRef = "subscription-model"
	entry.ModelDiscovery = catalog.ModelDiscovery{Method: catalog.ModelDiscoveryStaticCatalog, AliasPolicy: catalog.ModelAliasExactProviderID, Aliases: []catalog.ModelAlias{{Alias: "exact-001", ProviderModelRef: "subscription-model", Stable: true}}}
	entry.Evidence.Digest = ""
	digest, err := catalog.ComputeEvidenceDigest(entry)
	if err != nil {
		t.Fatal(err)
	}
	entry.Evidence.Digest = digest
	document.Entries = []catalog.Entry{entry}
	routeCatalog, err := catalog.New(document, testNow)
	if err != nil {
		t.Fatalf("construct callable subscription catalog: %v", err)
	}
	return routeCatalog, entry.Route
}

type trustedAuthorizationResolver struct {
	authorization modelinvoker.SubscriptionAuthorization
	calls         int
}

func (resolver *trustedAuthorizationResolver) ResolveSubscriptionAuthorization(_ context.Context, _ modelinvoker.SubscriptionAuthorizationRequest) (modelinvoker.SubscriptionAuthorization, error) {
	resolver.calls++
	return resolver.authorization, nil
}

func generalInvocation() upstream.InvocationContext {
	return upstream.InvocationContext{
		Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService,
		Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground,
	}
}

func assertInvokeErrorCode(t *testing.T, routed *modelinvoker.RouteInvoker, call modelinvoker.RouteCall, want string) {
	t.Helper()
	_, err := routed.Invoke(context.Background(), call)
	if err == nil {
		t.Fatalf("Invoke() error = nil, want code %q", want)
	}
	var routeError *modelinvoker.RouteError
	if !errors.As(err, &routeError) || routeError.RouteID == "" {
		t.Fatalf("error %T %v does not preserve RouteError", err, err)
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Code != want {
		t.Fatalf("error = %#v, want code %q", invocationError, want)
	}
}

type fakeProvider struct {
	id              modelinvoker.ProviderID
	capabilityCalls int
	invokeCalls     int
	streamCalls     int
	lastRequest     modelinvoker.Request
	stream          modelinvoker.Stream
}

func (provider *fakeProvider) ID() modelinvoker.ProviderID { return provider.id }

func (provider *fakeProvider) DefaultProtocol() modelinvoker.Protocol {
	return modelinvoker.ProtocolResponses
}

func (provider *fakeProvider) Capabilities(_ context.Context, _ modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	provider.capabilityCalls++
	contract := make(modelinvoker.CapabilityContract)
	for _, capability := range modelinvoker.AllCapabilities() {
		contract[capability] = modelinvoker.CapabilitySupport{Level: modelinvoker.SupportNative}
	}
	return contract, nil
}

func (provider *fakeProvider) Invoke(_ context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	provider.invokeCalls++
	provider.lastRequest = request
	return modelinvoker.Response{Status: modelinvoker.ResponseStatusCompleted, Model: request.Model}, nil
}

func (provider *fakeProvider) Stream(_ context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	provider.streamCalls++
	provider.lastRequest = request
	if provider.stream == nil {
		provider.stream = &fakeStream{}
	}
	return provider.stream, nil
}

type fakeStream struct {
	events      []modelinvoker.StreamEvent
	index       int
	closed      bool
	terminalErr error
}

func (stream *fakeStream) Next() bool {
	if stream.index >= len(stream.events) {
		return false
	}
	stream.index++
	return true
}

func (stream *fakeStream) Event() modelinvoker.StreamEvent {
	if stream.index == 0 || stream.index > len(stream.events) {
		return modelinvoker.StreamEvent{}
	}
	return stream.events[stream.index-1]
}

func (stream *fakeStream) Err() error { return stream.terminalErr }

func (stream *fakeStream) Close() error {
	stream.closed = true
	return nil
}
