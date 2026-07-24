package routegateway_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

var gatewayNow = time.Date(2026, 7, 18, 2, 30, 0, 0, time.UTC)

func TestEveryCallableRouteHasARealBuiltinConstructionPath(t *testing.T) {
	routeCatalog, err := catalog.NewDefault(gatewayNow)
	if err != nil {
		t.Fatal(err)
	}
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(factories.IDs()); got != 18 {
		t.Fatalf("built-in AdapterID count = %d, want 18", got)
	}
	gateway, err := routegateway.New(routeCatalog, routegateway.CatalogBindingResolver{}, catalogSecretResolver{}, factories, routegateway.WithClock(func() time.Time { return gatewayNow }))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := gateway.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	resolved := 0
	for _, entry := range routeCatalog.Entries() {
		if !entry.Implementation.Callable {
			continue
		}
		model := runtimeModel(entry)
		if entry.ModelDiscovery.Method == catalog.ModelDiscoveryStaticCatalog {
			model = entry.Route.Model.ProviderModelRef
			if len(entry.ModelDiscovery.Aliases) > 0 {
				model = entry.ModelDiscovery.Aliases[0].ProviderModelRef
			}
		}
		call := gatewayCallForEntry(entry, model)
		resolution, err := gateway.Resolve(context.Background(), call)
		if err != nil {
			t.Errorf("Resolve(%q) error = %v", entry.ID, err)
			continue
		}
		resolved++
		if resolution.Route.RouteID != entry.ID || resolution.FactoryID == "" || resolution.CredentialVersion == "" || resolution.BindingVersion == "" {
			t.Errorf("Resolve(%q) incomplete resolution = %#v", entry.ID, resolution)
		}
		capabilities, err := gateway.Capabilities(context.Background(), call)
		if err != nil {
			t.Errorf("Capabilities(%q) error = %v", entry.ID, err)
		} else if len(capabilities.Contract) == 0 {
			t.Errorf("Capabilities(%q) returned an empty contract", entry.ID)
		}
	}
	if resolved != 39 {
		t.Fatalf("real built-in construction paths = %d, want 39", resolved)
	}
}

func TestEveryHostBlockedSubscriptionCandidateConstructsOnlyWithTrustedResolver(t *testing.T) {
	blocked := make([]catalog.Entry, 0, 16)
	for _, entry := range catalog.DefaultDocument().Entries {
		if entry.Implementation.HostActivationRequirement == catalog.HostActivationTrustedSubscriptionAuthorizationResolver {
			blocked = append(blocked, entry)
		}
	}
	if len(blocked) != 16 {
		t.Fatalf("host-blocked subscription candidates = %d, want 16", len(blocked))
	}
	for _, candidate := range blocked {
		t.Run(string(candidate.ID), func(t *testing.T) {
			routeCatalog, entry := activatedSubscriptionCatalog(t, candidate.ID)
			factories, err := routegateway.NewBuiltinFactoryRegistry()
			if err != nil {
				t.Fatal(err)
			}
			state := &callState{}
			resolver := trustedSubscriptionResolver{state: state, authorization: authorizationFor(entry)}
			gateway, err := routegateway.New(
				routeCatalog, routegateway.CatalogBindingResolver{}, catalogSecretResolver{}, factories,
				routegateway.WithClock(func() time.Time { return gatewayNow }),
				routegateway.WithSubscriptionAuthorizationResolver(resolver),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer gateway.Close()
			model := entry.Route.Model.ProviderModelRef
			call := modelinvoker.RouteCall{RouteID: entry.ID, Request: modelinvoker.Request{Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "candidate construction")}}}
			result, err := gateway.Capabilities(context.Background(), call)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Contract) == 0 || result.Resolution.Route.Model != model || state.authorization.Load() != 1 {
				t.Fatalf("candidate capability resolution = %#v state=%#v", result, state.snapshot())
			}
		})
	}
}

func runtimeModel(entry catalog.Entry) string {
	switch entry.Implementation.AdapterID {
	case "deepseek":
		return "deepseek-v4-flash"
	case "kimi":
		return "kimi-k2.5"
	case "zai":
		return "glm-4.7"
	case "minimax":
		return "MiniMax-M3"
	case "xiaomi-mimo":
		return "mimo-v2.5"
	case "qwen":
		return "qwen3.7-max"
	case "xai":
		return "grok-4.5"
	case "azure-openai":
		return entry.Route.Deployment.DeploymentName
	case "kimi-code":
		return "kimi-for-coding"
	case "minimax-token-plan":
		return "MiniMax-M2.7"
	case "mimo-token-plan":
		return "mimo-v2.5"
	case "alibaba-plan":
		return "qwen3.7-max"
	default:
		return "runtime-selected-test-model"
	}
}

func gatewayCallForEntry(entry catalog.Entry, model string) modelinvoker.RouteCall {
	call := modelinvoker.RouteCall{RouteID: entry.ID, Invocation: generalInvocation(), Request: modelinvoker.Request{Model: model, Input: []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "offline construction")}}}
	if entry.Route.Offering.Kind != upstream.OfferingTokenPlan && entry.Route.Offering.Kind != upstream.OfferingCodingPlan {
		return call
	}
	remaining := int64(100)
	call.Invocation = upstream.InvocationContext{Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal, Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground, ClientIdentity: upstream.ClientIdentity{Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest}}
	call.EntitlementState = &upstream.EntitlementState{OfferingID: entry.Route.Offering.ID, CredentialProfile: entry.Route.Credential.ID, Status: upstream.EntitlementActive, ObservedAt: gatewayNow.Add(-time.Minute), ValidUntil: gatewayNow.Add(time.Minute), ExpiresAt: gatewayNow.Add(24 * time.Hour), RemainingQuota: &remaining}
	return call
}

type catalogSecretResolver struct{}

func (catalogSecretResolver) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	values := map[upstream.CredentialPurpose][]byte{}
	switch request.Profile.Type {
	case upstream.CredentialAPIKey:
		value := "offline-api-key"
		if len(request.Profile.KeyPrefixes) > 0 {
			value = request.Profile.KeyPrefixes[0] + "offline-api-key"
		}
		values[upstream.CredentialPurposeAPIKey] = []byte(value)
	case upstream.CredentialBearer:
		values[upstream.CredentialPurposeBearerToken] = []byte("offline-bearer-token")
	case upstream.CredentialSigV4:
		values[upstream.CredentialPurposeAccessKeyID] = []byte("AKIAOFFLINE")
		values[upstream.CredentialPurposeSecretAccessKey] = []byte("offline-secret-access-key")
		values[upstream.CredentialPurposeSessionToken] = []byte("offline-session-token")
	case upstream.CredentialADC, upstream.CredentialEntraID, upstream.CredentialOAuth:
		values[upstream.CredentialPurposeBearerToken] = []byte("offline-access-token")
	default:
		return routegateway.SecretMaterial{}, fmt.Errorf("unsupported credential type")
	}
	return routegateway.NewSecretMaterial(request.Profile.ID, request.Profile.Type, "offline-v1", gatewayNow.Add(time.Hour), values)
}

func generalInvocation() upstream.InvocationContext {
	return upstream.InvocationContext{Usage: upstream.InvocationGeneralAPI, Subject: upstream.SubjectService, Tenancy: upstream.TenancyMulti, Execution: upstream.ExecutionForeground}
}
