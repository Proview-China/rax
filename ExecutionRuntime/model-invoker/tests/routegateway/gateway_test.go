package routegateway_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestPolicyRejectionHappensBeforeBindingSecretFactoryAndProvider(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()

	var control catalog.Entry
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			control = entry
			break
		}
	}
	_, err := gateway.Invoke(context.Background(), modelinvoker.RouteCall{
		RouteID: control.ID, Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: control.Route.Model.ProviderModelRef, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "must reject")}},
	})
	if err == nil {
		t.Fatal("non-callable route error = nil")
	}
	if state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 || state.capabilities.Load() != 0 || state.invoke.Load() != 0 {
		t.Fatalf("policy rejection touched downstream: %#v", state.snapshot())
	}
}

func TestBindingAndSecretFailuresStopAtTheirExactBoundary(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	call := openAICall()

	state := &callState{}
	gateway := fakeGateway(t, routeCatalog, failingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	_, err := gateway.Resolve(context.Background(), call)
	if err == nil || state.binding.Load() != 1 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("binding boundary err=%v state=%#v", err, state.snapshot())
	}
	_ = gateway.Close()

	state = &callState{}
	gateway = fakeGateway(t, routeCatalog, countingBinding{state: state}, failingSecret{state: state}, state)
	_, err = gateway.Resolve(context.Background(), call)
	if err == nil || state.binding.Load() != 1 || state.secret.Load() != 1 || state.factory.Load() != 0 {
		t.Fatalf("secret boundary err=%v state=%#v", err, state.snapshot())
	}
	_ = gateway.Close()
}

func TestGatewayRejectsMismatchedSecretProfileAndTypedPurposeBeforeFactory(t *testing.T) {
	for _, test := range []struct{ name, mode, code string }{
		{"profile identity", "profile", "secret_profile_mismatch"},
		{"typed purpose", "purpose", "secret_value_missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := &callState{}
			gateway := fakeGateway(t, defaultCatalog(t), countingBinding{state: state}, invalidSecretResolver{state: state, mode: test.mode}, state)
			defer gateway.Close()
			_, err := gateway.Resolve(context.Background(), openAICall())
			if err == nil {
				t.Fatal("Resolve error = nil")
			}
			assertGatewayErrorCode(t, err, test.code)
			if state.factory.Load() != 0 || state.capabilities.Load() != 0 || state.invoke.Load() != 0 {
				t.Fatalf("invalid secret crossed Factory/Provider boundary: %#v", state.snapshot())
			}
		})
	}
}

func TestConcurrentResolveIsSingleflightAndRotationClosesOldAdapter(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	secret := &rotatingSecret{state: state, version: "rotation-v1"}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, secret, state)

	const workers = 32
	start := make(chan struct{})
	errorsByWorker := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := gateway.Resolve(context.Background(), openAICall())
			errorsByWorker <- err
		}()
	}
	close(start)
	wait.Wait()
	close(errorsByWorker)
	for err := range errorsByWorker {
		if err != nil {
			t.Fatalf("concurrent Resolve error = %v", err)
		}
	}
	if got := state.factory.Load(); got != 1 {
		t.Fatalf("factory calls after singleflight = %d, want 1", got)
	}

	secret.setVersion("rotation-v2")
	if _, err := gateway.Resolve(context.Background(), openAICall()); err != nil {
		t.Fatal(err)
	}
	if state.factory.Load() != 2 || state.closed.Load() != 1 {
		t.Fatalf("rotation state = %#v, want second factory and old close", state.snapshot())
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if state.closed.Load() != 2 {
		t.Fatalf("closed adapters = %d, want 2", state.closed.Load())
	}
}

func TestBindingVersionAndClientIdentityRotatePoolKeyAndCloseOldAdapterOnce(t *testing.T) {
	state := &callState{}
	bindings := &rotatingBinding{state: state, version: "binding-v1"}
	gateway := fakeGateway(t, defaultCatalog(t), bindings, countingSecret{state: state, version: "secret-v1"}, state)
	call := openAICall()
	call.Invocation.ClientIdentity = upstream.ClientIdentity{Name: "praxis-cli", Version: "v1", UserAgent: "praxis-cli/v1", Source: upstream.ClientIdentityRuntimeObserved}
	if _, err := gateway.Resolve(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	bindings.setVersion("binding-v2")
	if _, err := gateway.Resolve(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if state.factory.Load() != 2 || state.closed.Load() != 1 {
		t.Fatalf("binding rotation factory/close = %d/%d, want 2/1", state.factory.Load(), state.closed.Load())
	}
	call.Invocation.ClientIdentity.Version = "v2"
	call.Invocation.ClientIdentity.UserAgent = "praxis-cli/v2"
	if _, err := gateway.Resolve(context.Background(), call); err != nil {
		t.Fatal(err)
	}
	if state.factory.Load() != 3 || state.closed.Load() != 2 {
		t.Fatalf("client identity rotation factory/close = %d/%d, want 3/2", state.factory.Load(), state.closed.Load())
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if state.closed.Load() != 3 {
		t.Fatalf("final adapter close calls = %d, want 3", state.closed.Load())
	}
}

func TestStreamLeaseDefersAdapterCloseUntilStreamClose(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	if err := gateway.Close(); err != nil {
		t.Fatal(err)
	}
	if state.closed.Load() != 0 {
		t.Fatalf("adapter closed while stream lease active: %d", state.closed.Load())
	}
	if !stream.Next() || stream.Event().TextDelta != "ok" {
		t.Fatalf("stream event = %#v err=%v", stream.Event(), stream.Err())
	}
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if state.closed.Load() != 1 {
		t.Fatalf("adapter close count after stream close = %d", state.closed.Load())
	}
}

func TestGatewayRejectsProviderResponseModelDrift(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	state.wrongModel.Store(true)
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()

	result, err := gateway.Invoke(context.Background(), openAICall())
	if err == nil {
		t.Fatal("mismatched non-stream response model was accepted")
	} else {
		assertGatewayErrorCode(t, err, "response_model_mismatch")
	}
	if !reflect.DeepEqual(result.Response, modelinvoker.Response{}) {
		t.Fatalf("mismatched Invoke returned untrusted response: %#v", result.Response)
	}
	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Next() || stream.Event().Type != modelinvoker.StreamEventError || stream.Event().Error == nil {
		t.Fatalf("mismatched stream event = %#v", stream.Event())
	}
	if event := stream.Event(); event.Response != nil || !event.Raw.Empty() {
		t.Fatalf("mismatched stream exposed untrusted response/raw: %#v", event)
	}
	if stream.Next() {
		t.Fatal("mismatched model stream emitted events after its terminal error")
	}
	assertGatewayErrorCode(t, stream.Err(), "response_model_mismatch")
}

func TestGatewayRejectsMissingProviderResponseModel(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	state.missingModel.Store(true)
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()

	result, err := gateway.Invoke(context.Background(), openAICall())
	if err == nil {
		t.Fatal("missing non-stream response model was accepted")
	} else {
		assertGatewayErrorCode(t, err, "response_model_missing")
	}
	if !reflect.DeepEqual(result.Response, modelinvoker.Response{}) {
		t.Fatalf("missing-model Invoke returned untrusted response: %#v", result.Response)
	}
	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Next() || stream.Event().Type != modelinvoker.StreamEventError || stream.Event().Error == nil {
		t.Fatalf("missing-model stream event = %#v", stream.Event())
	}
	if event := stream.Event(); event.Response != nil || !event.Raw.Empty() {
		t.Fatalf("missing-model stream exposed untrusted response/raw: %#v", event)
	}
	if stream.Next() {
		t.Fatal("missing-model stream emitted events after its terminal error")
	}
	assertGatewayErrorCode(t, stream.Err(), "response_model_missing")
}

func TestGatewayStampsCatalogIdentityOverCustomFactoryResponse(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	state := &callState{}
	state.forgedIdentity.Store(true)
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()

	result, err := gateway.Invoke(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	assertGatewayStampedIdentity(t, result.Resolution, result.Response)

	stream, err := gateway.Stream(context.Background(), openAICall())
	if err != nil {
		t.Fatal(err)
	}
	if !stream.Next() || stream.Event().Response == nil {
		t.Fatalf("stream event = %#v", stream.Event())
	}
	assertGatewayStampedIdentity(t, stream.Resolution(), *stream.Event().Response)
}

func assertGatewayStampedIdentity(t *testing.T, resolution routegateway.Resolution, response modelinvoker.Response) {
	t.Helper()
	if response.Provider != resolution.Route.AdapterID || response.Protocol != resolution.Route.Protocol ||
		response.MappingReport.Provider != resolution.Route.AdapterID || response.MappingReport.Protocol != resolution.Route.Protocol ||
		response.MappingReport.Endpoint != resolution.Route.Endpoint {
		t.Fatalf("Gateway response identity = %#v, resolution = %#v", response, resolution)
	}
	if response.State == nil || response.State.Provider != resolution.Route.AdapterID || response.State.Protocol != resolution.Route.Protocol ||
		response.State.Kind != modelinvoker.StateProviderContinuation || response.State.ID != "provider-state" || string(response.State.Payload.Bytes()) != "opaque-state" {
		t.Fatalf("Gateway response state identity/payload = %#v", response.State)
	}
	retained := false
	for _, decision := range response.MappingReport.Decisions {
		retained = retained || decision.Detail == "provider decision retained"
	}
	if !retained {
		t.Fatalf("Gateway discarded provider mapping decisions: %#v", response.MappingReport.Decisions)
	}
}

func TestSecretMaterialFormattingCannotRevealSecret(t *testing.T) {
	secret := "DO-NOT-LEAK-THIS-SECRET"
	material, err := routegateway.NewSecretMaterial("test.profile", upstream.CredentialAPIKey, "version-not-secret", time.Time{}, map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeAPIKey: []byte(secret)})
	if err != nil {
		t.Fatal(err)
	}
	formatted := fmt.Sprintf("%v %#v %+v", material, material, material)
	if strings.Contains(formatted, secret) || !strings.Contains(formatted, "REDACTED") {
		t.Fatalf("unsafe SecretMaterial formatting: %s", formatted)
	}
}

func TestDefaultSubscriptionRouteIsBlockedByHostTrustBeforeDownstream(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	entry, ok := routeCatalog.Get("kimi.code-membership.global.chat_completions")
	if !ok || entry.Implementation.Callable || entry.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
		t.Fatal("Kimi Code route is not blocked by host trust")
	}
	state := &callState{}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()
	call := modelinvoker.RouteCall{RouteID: entry.ID, Request: modelinvoker.Request{Model: "kimi-for-coding", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}}
	remaining := int64(100)
	call.Invocation = trustedInvocation()
	call.EntitlementState = &upstream.EntitlementState{OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID, Status: upstream.EntitlementActive, ObservedAt: gatewayNow.Add(-time.Minute), ValidUntil: gatewayNow.Add(time.Minute), RemainingQuota: &remaining}
	if _, err := gateway.Resolve(context.Background(), call); err == nil {
		t.Fatal("host-blocked subscription route error = nil")
	}
	if state.authorization.Load() != 0 || state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("host-blocked subscription touched downstream: %#v", state.snapshot())
	}
}

func TestTrustedSubscriptionResolverIsTheOnlyClaimUpgradePath(t *testing.T) {
	routeCatalog, entry := activatedSubscriptionCatalog(t, "kimi.code-membership.global.chat_completions")
	state := &callState{}
	if _, err := fakeGatewayResult(routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state); err == nil {
		t.Fatal("callable subscription catalog without trusted resolver constructed a Gateway")
	}
	resolver := trustedSubscriptionResolver{state: state, authorization: authorizationFor(entry)}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state, routegateway.WithSubscriptionAuthorizationResolver(resolver))
	defer gateway.Close()
	base := modelinvoker.RouteCall{RouteID: entry.ID, Request: modelinvoker.Request{Model: "kimi-for-coding", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}}
	for name, forged := range map[string]modelinvoker.RouteCall{
		"build_manifest": func() modelinvoker.RouteCall { value := base; value.Invocation = trustedInvocation(); return value }(),
		"runtime_observed": func() modelinvoker.RouteCall {
			value := base
			value.Invocation = trustedInvocation()
			value.Invocation.ClientIdentity.Source = upstream.ClientIdentityRuntimeObserved
			return value
		}(),
		"active_entitlement": func() modelinvoker.RouteCall {
			value := base
			entitlement := authorizationFor(entry).Entitlement
			value.EntitlementState = &entitlement
			return value
		}(),
	} {
		if _, err := gateway.Resolve(context.Background(), forged); err == nil {
			t.Errorf("forged %s claim error = nil", name)
		}
	}
	if state.authorization.Load() != 0 || state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("forged claims touched trust/downstream boundaries: %#v", state.snapshot())
	}
	if _, err := gateway.Resolve(context.Background(), base); err != nil {
		t.Fatalf("trusted subscription resolution failed: %v", err)
	}
	if state.authorization.Load() != 1 || state.binding.Load() != 1 || state.secret.Load() != 1 || state.factory.Load() != 1 {
		t.Fatalf("trusted resolution did not traverse each boundary exactly once: %#v", state.snapshot())
	}
}

func TestEveryRestrictedSubscriptionOfferingRejectsUnknownModelBeforeTrustAndSecret(t *testing.T) {
	for _, routeID := range []upstream.RouteID{
		"kimi.code-membership.global.chat_completions",
		"minimax.token-plan.global.chat_completions",
		"mimo.token-plan.cn.chat_completions",
		"alibaba.coding-plan.cn.chat_completions",
		"alibaba.token-plan-team.cn-beijing.chat_completions",
	} {
		t.Run(string(routeID), func(t *testing.T) {
			routeCatalog, entry := activatedSubscriptionCatalog(t, routeID)
			state := &callState{}
			resolver := trustedSubscriptionResolver{state: state, authorization: authorizationFor(entry)}
			gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state, routegateway.WithSubscriptionAuthorizationResolver(resolver))
			defer gateway.Close()
			call := modelinvoker.RouteCall{RouteID: routeID, Request: modelinvoker.Request{Model: "definitely-not-an-approved-model", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}}
			if _, err := gateway.Resolve(context.Background(), call); err == nil {
				t.Fatal("invalid model error = nil")
			}
			if state.authorization.Load() != 0 || state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
				t.Fatalf("invalid model touched trust/downstream: %#v", state.snapshot())
			}
		})
	}
}

func TestTenDirectProviderFamiliesRejectUnknownModelBeforeBindingAndSecret(t *testing.T) {
	routeIDs := []upstream.RouteID{
		"openai.direct.payg.responses",
		"anthropic.direct.payg.messages",
		"google.gemini-developer.payg.generate_content",
		"xai.api.global.payg.responses",
		"zai.platform.global.payg.chat_completions",
		"deepseek.direct.payg.chat_completions",
		"kimi.platform.global.payg.chat_completions",
		"minimax.platform.global.payg.messages",
		"xiaomi.mimo.global.payg.messages",
		"alibaba.model-studio.cn-beijing.payg.responses",
	}
	for _, routeID := range routeIDs {
		t.Run(string(routeID), func(t *testing.T) {
			state := &callState{}
			gateway := fakeGateway(t, defaultCatalog(t), countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
			defer gateway.Close()
			_, err := gateway.Resolve(context.Background(), modelinvoker.RouteCall{
				RouteID: routeID, Invocation: generalInvocation(),
				Request: modelinvoker.Request{Model: "definitely-not-an-approved-model", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
			})
			if err == nil {
				t.Fatal("unknown model error = nil")
			}
			if state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 || state.capabilities.Load() != 0 {
				t.Fatalf("unknown model touched downstream: %#v", state.snapshot())
			}
		})
	}
}

func TestGLMCodingPlanRemainsNonCallableForPraxis(t *testing.T) {
	routeCatalog := defaultCatalog(t)
	entry, ok := routeCatalog.Get("zai.glm-coding-plan.cn.chat_completions")
	if !ok || entry.Implementation.Callable {
		t.Fatal("GLM Coding Plan boundary drifted")
	}
	state := &callState{}
	gateway := fakeGateway(t, routeCatalog, countingBinding{state: state}, countingSecret{state: state, version: "v1"}, state)
	defer gateway.Close()
	remaining := int64(100)
	call := modelinvoker.RouteCall{RouteID: entry.ID,
		Invocation:       upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal, Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground, ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest}},
		EntitlementState: &upstream.EntitlementState{OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID, Status: upstream.EntitlementActive, ObservedAt: gatewayNow.Add(-time.Minute), ValidUntil: gatewayNow.Add(time.Minute), RemainingQuota: &remaining},
		Request:          modelinvoker.Request{Model: "GLM-4.7", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	}
	if _, err := gateway.Resolve(context.Background(), call); err == nil {
		t.Fatal("GLM Coding Plan error = nil")
	}
	if state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("blocked GLM route touched downstream: %#v", state.snapshot())
	}
}

type callState struct {
	authorization, binding, secret, factory, capabilities, invoke, stream, closed atomic.Int64
	wrongModel, missingModel, forgedIdentity                                      atomic.Bool
}

func (s *callState) snapshot() map[string]int64 {
	return map[string]int64{"authorization": s.authorization.Load(), "binding": s.binding.Load(), "secret": s.secret.Load(), "factory": s.factory.Load(), "capabilities": s.capabilities.Load(), "invoke": s.invoke.Load(), "stream": s.stream.Load(), "closed": s.closed.Load()}
}

type countingBinding struct{ state *callState }

func (r countingBinding) ResolveBinding(ctx context.Context, request routegateway.BindingRequest) (routegateway.RuntimeBinding, error) {
	r.state.binding.Add(1)
	return routegateway.CatalogBindingResolver{}.ResolveBinding(ctx, request)
}

type rotatingBinding struct {
	state   *callState
	mu      sync.RWMutex
	version string
}

func (resolver *rotatingBinding) setVersion(version string) {
	resolver.mu.Lock()
	resolver.version = version
	resolver.mu.Unlock()
}

func (resolver *rotatingBinding) ResolveBinding(ctx context.Context, request routegateway.BindingRequest) (routegateway.RuntimeBinding, error) {
	resolver.state.binding.Add(1)
	binding, err := (routegateway.CatalogBindingResolver{}).ResolveBinding(ctx, request)
	if err != nil {
		return routegateway.RuntimeBinding{}, err
	}
	resolver.mu.RLock()
	binding.Version = resolver.version
	resolver.mu.RUnlock()
	return binding, nil
}

type failingBinding struct{ state *callState }

func (r failingBinding) ResolveBinding(context.Context, routegateway.BindingRequest) (routegateway.RuntimeBinding, error) {
	r.state.binding.Add(1)
	return routegateway.RuntimeBinding{}, errors.New("binding unavailable")
}

type countingSecret struct {
	state   *callState
	version string
}

func (r countingSecret) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	r.state.secret.Add(1)
	return testMaterial(request, r.version)
}

type failingSecret struct{ state *callState }

func (r failingSecret) ResolveSecret(context.Context, routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	r.state.secret.Add(1)
	return routegateway.SecretMaterial{}, errors.New("resolver details must not escape")
}

type invalidSecretResolver struct {
	state *callState
	mode  string
}

func (resolver invalidSecretResolver) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	resolver.state.secret.Add(1)
	profileID := request.Profile.ID
	values := map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeAPIKey: []byte("offline-api-key")}
	if resolver.mode == "profile" {
		profileID = "wrong.profile"
	}
	if resolver.mode == "purpose" {
		values = map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeBearerToken: []byte("offline-bearer")}
	}
	return routegateway.NewSecretMaterial(profileID, request.Profile.Type, "invalid-secret-v1", gatewayNow.Add(time.Hour), values)
}

type rotatingSecret struct {
	state   *callState
	mu      sync.RWMutex
	version string
}

func (r *rotatingSecret) setVersion(version string) { r.mu.Lock(); r.version = version; r.mu.Unlock() }
func (r *rotatingSecret) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	r.state.secret.Add(1)
	r.mu.RLock()
	version := r.version
	r.mu.RUnlock()
	return testMaterial(request, version)
}

func testMaterial(request routegateway.SecretRequest, version string) (routegateway.SecretMaterial, error) {
	value := "offline-api-key"
	if len(request.Profile.KeyPrefixes) > 0 {
		value = request.Profile.KeyPrefixes[0] + "offline"
	}
	return routegateway.NewSecretMaterial(request.Profile.ID, request.Profile.Type, version, gatewayNow.Add(time.Hour), map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeAPIKey: []byte(value)})
}

type fakeFactory struct {
	id    modelinvoker.ProviderID
	state *callState
}

func (f fakeFactory) ID() string                         { return "fake/" + string(f.id) }
func (fakeFactory) Version() string                      { return "v1" }
func (f fakeFactory) AdapterID() modelinvoker.ProviderID { return f.id }
func (f fakeFactory) Build(_ context.Context, input routegateway.FactoryInput) (routegateway.FactoryResult, error) {
	f.state.factory.Add(1)
	time.Sleep(time.Millisecond)
	return routegateway.FactoryResult{Provider: &fakeProvider{id: f.id, state: f.state}, Closer: countCloser{state: f.state}, Endpoint: input.Endpoint}, nil
}

type countCloser struct{ state *callState }

func (c countCloser) Close() error { c.state.closed.Add(1); return nil }

type fakeProvider struct {
	id    modelinvoker.ProviderID
	state *callState
}

func (p *fakeProvider) ID() modelinvoker.ProviderID          { return p.id }
func (*fakeProvider) DefaultProtocol() modelinvoker.Protocol { return modelinvoker.ProtocolResponses }
func (p *fakeProvider) Capabilities(_ context.Context, query modelinvoker.CapabilityQuery) (modelinvoker.CapabilityContract, error) {
	p.state.capabilities.Add(1)
	contract := make(modelinvoker.CapabilityContract)
	for _, capability := range []modelinvoker.Capability{modelinvoker.CapabilityTextGeneration, modelinvoker.CapabilityStreaming, modelinvoker.CapabilityToolCalling, modelinvoker.CapabilityParallelToolCalling, modelinvoker.CapabilityStructuredOutput, modelinvoker.CapabilityReasoning, modelinvoker.CapabilityReasoningSummary, modelinvoker.CapabilityFunctionErrorResult, modelinvoker.CapabilityServerState, modelinvoker.CapabilityProviderContinuation, modelinvoker.CapabilityPromptCaching, modelinvoker.CapabilityUsageReporting} {
		contract[capability] = modelinvoker.CapabilitySupport{Level: modelinvoker.SupportCompatible, Models: []string{query.Model}, Protocols: []modelinvoker.Protocol{query.Protocol}}
	}
	return contract, nil
}
func (p *fakeProvider) Invoke(_ context.Context, request modelinvoker.Request) (modelinvoker.Response, error) {
	p.state.invoke.Add(1)
	model := request.Model
	if p.state.missingModel.Load() {
		model = ""
	} else if p.state.wrongModel.Load() {
		model = "mapped-to-another-model"
	}
	provider, protocol, endpoint := p.id, request.Protocol, request.Endpoint
	if p.state.forgedIdentity.Load() {
		provider, protocol, endpoint = "forged-provider", modelinvoker.ProtocolMessages, "https://forged.invalid/v1"
	}
	return modelinvoker.Response{Provider: provider, Protocol: protocol, Model: model, Output: []modelinvoker.OutputItem{{Type: modelinvoker.OutputItemText, Text: "ok"}}, RawResponse: modelinvoker.NewRawPayload([]byte("untrusted-provider-response")), State: &modelinvoker.State{Kind: modelinvoker.StateProviderContinuation, Provider: provider, Protocol: protocol, ID: "provider-state", Payload: modelinvoker.NewRawPayload([]byte("opaque-state"))}, MappingReport: modelinvoker.MappingReport{Provider: provider, Protocol: protocol, Endpoint: endpoint, Decisions: []modelinvoker.MappingDecision{{Detail: "provider decision retained"}}}}, nil
}
func (p *fakeProvider) Stream(_ context.Context, request modelinvoker.Request) (modelinvoker.Stream, error) {
	p.state.stream.Add(1)
	model := request.Model
	if p.state.missingModel.Load() {
		model = ""
	} else if p.state.wrongModel.Load() {
		model = "mapped-to-another-model"
	}
	return &fakeStream{model: model, forgeIdentity: p.state.forgedIdentity.Load(), provider: p.id, protocol: request.Protocol, endpoint: request.Endpoint}, nil
}

type fakeStream struct {
	sent, closed  bool
	model         string
	forgeIdentity bool
	provider      modelinvoker.ProviderID
	protocol      modelinvoker.Protocol
	endpoint      string
}

func (s *fakeStream) Next() bool {
	if s.closed || s.sent {
		return false
	}
	s.sent = true
	return true
}
func (s *fakeStream) Event() modelinvoker.StreamEvent {
	provider, protocol, endpoint := s.provider, s.protocol, s.endpoint
	if s.forgeIdentity {
		provider, protocol, endpoint = "forged-provider", modelinvoker.ProtocolMessages, "https://forged.invalid/v1"
	}
	return modelinvoker.StreamEvent{Type: modelinvoker.StreamEventTextDelta, TextDelta: "ok", Response: &modelinvoker.Response{Provider: provider, Protocol: protocol, Model: s.model, RawResponse: modelinvoker.NewRawPayload([]byte("untrusted-provider-response")), State: &modelinvoker.State{Kind: modelinvoker.StateProviderContinuation, Provider: provider, Protocol: protocol, ID: "provider-state", Payload: modelinvoker.NewRawPayload([]byte("opaque-state"))}, MappingReport: modelinvoker.MappingReport{Provider: provider, Protocol: protocol, Endpoint: endpoint, Decisions: []modelinvoker.MappingDecision{{Detail: "provider decision retained"}}}}, Raw: modelinvoker.NewRawPayload([]byte("untrusted-provider-event"))}
}
func (*fakeStream) Err() error     { return nil }
func (s *fakeStream) Close() error { s.closed = true; return nil }

func fakeGateway(t *testing.T, routeCatalog *catalog.Catalog, bindings routegateway.RuntimeBindingResolver, secrets routegateway.SecretResolver, state *callState, options ...routegateway.Option) *routegateway.Gateway {
	t.Helper()
	gateway, err := fakeGatewayResult(routeCatalog, bindings, secrets, state, options...)
	if err != nil {
		t.Fatal(err)
	}
	return gateway
}

func fakeGatewayResult(routeCatalog *catalog.Catalog, bindings routegateway.RuntimeBindingResolver, secrets routegateway.SecretResolver, state *callState, options ...routegateway.Option) (*routegateway.Gateway, error) {
	builtins, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		return nil, err
	}
	factories := make([]routegateway.AdapterFactory, 0, len(builtins.IDs()))
	for _, id := range builtins.IDs() {
		factories = append(factories, fakeFactory{id: id, state: state})
	}
	registry, err := routegateway.NewFactoryRegistry(factories...)
	if err != nil {
		return nil, err
	}
	options = append([]routegateway.Option{routegateway.WithClock(func() time.Time { return gatewayNow })}, options...)
	return routegateway.New(routeCatalog, bindings, secrets, registry, options...)
}

func defaultCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	value, err := catalog.NewDefault(gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func openAICall() modelinvoker.RouteCall {
	return modelinvoker.RouteCall{RouteID: "openai.direct.payg.responses", Invocation: generalInvocation(), Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}}}
}

func activatedSubscriptionCatalog(t *testing.T, routeID upstream.RouteID) (*catalog.Catalog, catalog.Entry) {
	t.Helper()
	document := catalog.DefaultDocument()
	var activated catalog.Entry
	for index := range document.Entries {
		if document.Entries[index].ID != routeID {
			continue
		}
		document.Entries[index].Implementation.Callable = true
		document.Entries[index].Implementation.HostActivationRequirement = ""
		activated = document.Entries[index].Clone()
		break
	}
	if activated.ID == "" {
		t.Fatalf("subscription route %q not found", routeID)
	}
	routeCatalog, err := catalog.New(document, gatewayNow)
	if err != nil {
		t.Fatalf("activate subscription route %q: %v", routeID, err)
	}
	return routeCatalog, activated
}

func trustedInvocation() upstream.InvocationContext {
	return upstream.InvocationContext{
		Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal,
		Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground,
		ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest},
	}
}

func authorizationFor(entry catalog.Entry) modelinvoker.SubscriptionAuthorization {
	remaining := int64(100)
	return modelinvoker.SubscriptionAuthorization{
		Invocation: trustedInvocation(),
		Entitlement: upstream.EntitlementState{
			OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID,
			Status: upstream.EntitlementActive, ObservedAt: gatewayNow.Add(-time.Minute), ValidUntil: gatewayNow.Add(time.Minute),
			ExpiresAt: gatewayNow.Add(24 * time.Hour), RemainingQuota: &remaining,
		},
	}
}

type trustedSubscriptionResolver struct {
	state         *callState
	authorization modelinvoker.SubscriptionAuthorization
}

func (resolver trustedSubscriptionResolver) ResolveSubscriptionAuthorization(_ context.Context, _ modelinvoker.SubscriptionAuthorizationRequest) (modelinvoker.SubscriptionAuthorization, error) {
	resolver.state.authorization.Add(1)
	return resolver.authorization, nil
}

var _ io.Closer = countCloser{}
