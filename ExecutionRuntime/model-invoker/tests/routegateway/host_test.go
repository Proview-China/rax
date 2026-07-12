package routegateway_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
)

func TestNewHostKeepsDefaultCatalogFailClosed(t *testing.T) {
	base := defaultCatalog(t)
	state := &callState{}
	gateway, report, err := routegateway.NewHost(hostConfig(t, base, state))
	if err != nil {
		t.Fatalf("NewHost(default) error = %v", err)
	}
	defer gateway.Close()
	if !report.Ready || report.FailureCode != "" || report.Activation.Applied || len(report.ActivatedSubscriptionRouteIDs) != 0 || len(report.CallableRouteIDs) != 39 || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("default host report = %#v", report)
	}
	entry, ok := base.Get("kimi.code-membership.global.chat_completions")
	if !ok {
		t.Fatal("Kimi Code route missing")
	}
	_, err = gateway.Resolve(context.Background(), modelinvoker.RouteCall{
		RouteID: entry.ID,
		Request: modelinvoker.Request{Model: "kimi-for-coding", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	})
	assertGatewayErrorCode(t, err, "route_not_callable")
	if state.authorization.Load() != 0 || state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("fail-closed host touched downstream: %#v", state.snapshot())
	}
}

func TestNewHostAtomicallyActivatesExactSubscriptionRoute(t *testing.T) {
	base := defaultCatalog(t)
	entry, ok := base.Get("kimi.code-membership.global.chat_completions")
	if !ok {
		t.Fatal("Kimi Code route missing")
	}
	state := &callState{}
	config := hostConfig(t, base, state)
	config.ActivationPlan = activationPlan(entry)
	config.SubscriptionAuthorizationResolver = trustedSubscriptionResolver{state: state, authorization: authorizationFor(entry)}

	gateway, report, err := routegateway.NewHost(config)
	if err != nil {
		t.Fatalf("NewHost(activated) error = %v", err)
	}
	defer gateway.Close()
	if !report.Ready || report.FailureCode != "" || !report.Activation.Applied || len(report.ActivatedSubscriptionRouteIDs) != 1 || report.ActivatedSubscriptionRouteIDs[0] != entry.ID || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("activated host report = %#v", report)
	}
	_, err = gateway.Resolve(context.Background(), modelinvoker.RouteCall{
		RouteID: entry.ID,
		Request: modelinvoker.Request{Model: "kimi-for-coding", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	})
	if err != nil {
		t.Fatalf("activated Resolve() error = %v", err)
	}
	if state.authorization.Load() != 1 || state.binding.Load() != 1 || state.secret.Load() != 1 || state.factory.Load() != 1 {
		t.Fatalf("activated host boundary counts = %#v", state.snapshot())
	}
	blocked, _ := base.Get(entry.ID)
	if blocked.Implementation.Callable || blocked.Implementation.HostActivationRequirement != catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
		t.Fatal("NewHost mutated BaseCatalog")
	}
}

func TestNewHostActivationRequiresResolverAndFactoryBeforeGateway(t *testing.T) {
	base := defaultCatalog(t)
	entry, _ := base.Get("kimi.code-membership.global.chat_completions")
	state := &callState{}
	config := hostConfig(t, base, state)
	config.ActivationPlan = activationPlan(entry)

	gateway, report, err := routegateway.NewHost(config)
	if gateway != nil || err == nil || report.Ready || !report.Activation.Applied || report.FailureCode != "host_subscription_authorization_required" || len(report.CallableRouteIDs) != 40 || len(report.ActivatedSubscriptionRouteIDs) != 1 || report.ActivatedSubscriptionRouteIDs[0] != entry.ID || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("missing resolver result gateway=%v report=%#v err=%v", gateway, report, err)
	}
	assertGatewayErrorCode(t, err, "host_subscription_authorization_required")
	var typedNil *typedNilHostResolver
	config.SubscriptionAuthorizationResolver = typedNil
	gateway, report, err = routegateway.NewHost(config)
	if gateway != nil || err == nil || report.Ready || report.FailureCode != "host_subscription_authorization_required" || len(report.CallableRouteIDs) != 40 || len(report.ActivatedSubscriptionRouteIDs) != 1 {
		t.Fatalf("typed-nil resolver result gateway=%v report=%#v err=%v", gateway, report, err)
	}
	assertGatewayErrorCode(t, err, "host_subscription_authorization_required")

	config.SubscriptionAuthorizationResolver = trustedSubscriptionResolver{state: state, authorization: authorizationFor(entry)}
	config.Factories = fakeFactoryRegistry(t, state, "kimi-code")
	gateway, report, err = routegateway.NewHost(config)
	if gateway != nil || err == nil || report.Ready || report.FailureCode != "host_callable_factory_missing" || len(report.CallableRouteIDs) != 40 || len(report.ActivatedSubscriptionRouteIDs) != 1 {
		t.Fatalf("missing factory result gateway=%v report=%#v err=%v", gateway, report, err)
	}
	assertGatewayErrorCode(t, err, "host_callable_factory_missing")
	if state.authorization.Load() != 0 || state.binding.Load() != 0 || state.secret.Load() != 0 || state.factory.Load() != 0 {
		t.Fatalf("failed host construction touched runtime dependencies: %#v", state.snapshot())
	}
}

func TestNewHostRejectsUnauditedPreactivatedSubscription(t *testing.T) {
	base, _ := activatedSubscriptionCatalog(t, "kimi.code-membership.global.chat_completions")
	state := &callState{}
	config := hostConfig(t, base, state)
	config.SubscriptionAuthorizationResolver = trustedSubscriptionResolver{state: state}
	gateway, report, err := routegateway.NewHost(config)
	if gateway != nil || err == nil || report.Ready || report.FailureCode != "host_activation_plan_required" || len(report.CallableRouteIDs) != 40 || len(report.ActivatedSubscriptionRouteIDs) != 1 || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("unaudited preactivation result gateway=%v report=%#v err=%v", gateway, report, err)
	}
	assertGatewayErrorCode(t, err, "host_activation_plan_required")
}

func TestNewHostReportsExactDisabledRoute(t *testing.T) {
	base := defaultCatalog(t)
	entry, ok := base.Get("openai.direct.payg.chat_completions")
	if !ok {
		t.Fatal("OpenAI Chat route missing")
	}
	state := &callState{}
	config := hostConfig(t, base, state)
	config.ActivationPlan = &catalog.ActivationPlan{
		ID: "disable-openai-chat", Revision: "r1",
		Routes: []catalog.RouteActivation{{RouteID: entry.ID, Action: catalog.DisableRoute, ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID}},
	}
	gateway, report, err := routegateway.NewHost(config)
	if err != nil {
		t.Fatalf("NewHost(disable) error = %v", err)
	}
	defer gateway.Close()
	if !report.Ready || len(report.DisabledRouteIDs) != 1 || report.DisabledRouteIDs[0] != entry.ID || len(report.CallableRouteIDs) != 38 {
		t.Fatalf("disabled host report = %#v", report)
	}
	_, err = gateway.Resolve(context.Background(), modelinvoker.RouteCall{
		RouteID: entry.ID, Invocation: generalInvocation(),
		Request: modelinvoker.Request{Model: "gpt-5.5", Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "hello")}},
	})
	assertGatewayErrorCode(t, err, "route_not_callable")
}

func TestNewHostCanDisableUnauditedPreactivatedSubscription(t *testing.T) {
	base, entry := activatedSubscriptionCatalog(t, "kimi.code-membership.global.chat_completions")
	state := &callState{}
	config := hostConfig(t, base, state)
	config.ActivationPlan = &catalog.ActivationPlan{
		ID: "disable-unaudited-kimi", Revision: "r1",
		Routes: []catalog.RouteActivation{{
			RouteID: entry.ID, Action: catalog.DisableRoute,
			ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID,
		}},
	}
	gateway, report, err := routegateway.NewHost(config)
	if err != nil {
		t.Fatalf("NewHost(disable preactivated) error = %v", err)
	}
	defer gateway.Close()
	if !report.Ready || len(report.ActivatedSubscriptionRouteIDs) != 0 || len(report.DisabledRouteIDs) != 1 || report.DisabledRouteIDs[0] != entry.ID || len(report.CallableRouteIDs) != 39 {
		t.Fatalf("disable preactivated report = %#v", report)
	}
}

func TestNewHostRejectsTypedNilBindingAndSecretResolvers(t *testing.T) {
	base := defaultCatalog(t)
	state := &callState{}
	config := hostConfig(t, base, state)
	var binding *typedNilBindingResolver
	config.BindingResolver = binding
	if gateway, report, err := routegateway.NewHost(config); gateway != nil || err == nil || report.FailureCode != "host_config_required" || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("typed-nil binding result gateway=%v report=%#v err=%v", gateway, report, err)
	}
	config = hostConfig(t, base, state)
	var secret *typedNilSecretResolver
	config.SecretResolver = secret
	if gateway, report, err := routegateway.NewHost(config); gateway != nil || err == nil || report.FailureCode != "host_config_required" || !validHostAuditDigest(report.AuditDigest) {
		t.Fatalf("typed-nil secret result gateway=%v report=%#v err=%v", gateway, report, err)
	}
}

func hostConfig(t *testing.T, base *catalog.Catalog, state *callState) routegateway.HostConfig {
	t.Helper()
	return routegateway.HostConfig{
		BaseCatalog: base, BindingResolver: countingBinding{state: state}, SecretResolver: countingSecret{state: state, version: "v1"},
		Factories: fakeFactoryRegistry(t, state, ""), Clock: func() time.Time { return gatewayNow },
	}
}

func activationPlan(entry catalog.Entry) *catalog.ActivationPlan {
	return &catalog.ActivationPlan{
		ID: "activate-kimi-code", Revision: "r1",
		Routes: []catalog.RouteActivation{{
			RouteID: entry.ID, Action: catalog.ActivateHostBlockedRoute,
			ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID,
		}},
	}
}

func fakeFactoryRegistry(t *testing.T, state *callState, excluded modelinvoker.ProviderID) *routegateway.FactoryRegistry {
	t.Helper()
	builtins, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	factories := make([]routegateway.AdapterFactory, 0, len(builtins.IDs()))
	for _, id := range builtins.IDs() {
		if id == excluded {
			continue
		}
		factories = append(factories, fakeFactory{id: id, state: state})
	}
	registry, err := routegateway.NewFactoryRegistry(factories...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func assertGatewayErrorCode(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want code %q", want)
	}
	var invocationError *modelinvoker.Error
	if !errors.As(err, &invocationError) || invocationError.Code != want {
		t.Fatalf("error = %#v, want code %q", invocationError, want)
	}
}

func validHostAuditDigest(value string) bool {
	return len(value) == len("sha256:")+64 && strings.HasPrefix(value, "sha256:")
}

var _ modelinvoker.SubscriptionAuthorizationResolver = (*typedNilHostResolver)(nil)

type typedNilHostResolver struct{}

func (*typedNilHostResolver) ResolveSubscriptionAuthorization(context.Context, modelinvoker.SubscriptionAuthorizationRequest) (modelinvoker.SubscriptionAuthorization, error) {
	return modelinvoker.SubscriptionAuthorization{}, nil
}

type typedNilBindingResolver struct{}

func (*typedNilBindingResolver) ResolveBinding(context.Context, routegateway.BindingRequest) (routegateway.RuntimeBinding, error) {
	return routegateway.RuntimeBinding{}, nil
}

type typedNilSecretResolver struct{}

func (*typedNilSecretResolver) ResolveSecret(context.Context, routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	return routegateway.SecretMaterial{}, nil
}
