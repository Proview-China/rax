//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/catalog"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/routegateway"
	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

const subscriptionSmokeMarker = "praxis-subscription-ok"

func TestSubscriptionRoutesLiveSmoke(t *testing.T) {
	tests := []struct {
		name       string
		enableEnv  string
		keyEnv     string
		routeEnv   string
		modelEnv   string
		allowedIDs []upstream.RouteID
	}{
		{
			name: "kimi_code", enableEnv: "PRAXIS_KIMI_CODE_LIVE_TESTS", keyEnv: "KIMI_CODE_API_KEY",
			routeEnv: "KIMI_CODE_SMOKE_ROUTE_ID", modelEnv: "KIMI_CODE_SMOKE_MODEL",
			allowedIDs: []upstream.RouteID{"kimi.code-membership.global.chat_completions", "kimi.code-membership.global.messages"},
		},
		{
			name: "minimax_token_plan", enableEnv: "PRAXIS_MINIMAX_TOKEN_PLAN_LIVE_TESTS", keyEnv: "MINIMAX_TOKEN_PLAN_API_KEY",
			routeEnv: "MINIMAX_TOKEN_PLAN_SMOKE_ROUTE_ID", modelEnv: "MINIMAX_TOKEN_PLAN_SMOKE_MODEL",
			allowedIDs: []upstream.RouteID{"minimax.token-plan.global.chat_completions", "minimax.token-plan.global.messages"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if os.Getenv("PRAXIS_LIVE_TESTS") != "1" || os.Getenv(test.enableEnv) != "1" {
				t.Skip("subscription smoke requires global and route-family confirmations")
			}
			key, routeID, model := os.Getenv(test.keyEnv), upstream.RouteID(os.Getenv(test.routeEnv)), os.Getenv(test.modelEnv)
			if err := validateSubscriptionSmokeInputs(key, routeID, model); err != nil {
				t.Fatalf("enabled subscription smoke requires %s, %s, and %s: %v", test.keyEnv, test.routeEnv, test.modelEnv, err)
			}
			if !slices.Contains(test.allowedIDs, routeID) {
				t.Fatalf("%s=%q is outside the reviewed exact RouteID set", test.routeEnv, routeID)
			}
			runSubscriptionRouteSmoke(t, routeID, model, test.keyEnv, key)
		})
	}
}

func runSubscriptionRouteSmoke(t *testing.T, routeID upstream.RouteID, model, keyEnv, key string) {
	t.Helper()
	now := time.Now().UTC()
	base, err := catalog.NewDefault(now)
	if err != nil {
		t.Fatalf("construct fresh default catalog: %v", err)
	}
	entry, ok := base.Get(routeID)
	if !ok {
		t.Fatalf("RouteID %q is not present in the default catalog", routeID)
	}
	if len(entry.Route.Credential.References) != 1 ||
		entry.Route.Credential.References[0] != (upstream.CredentialReference{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: keyEnv}) {
		t.Fatal("selected subscription Route does not match the reviewed live credential reference")
	}
	plan := catalog.ActivationPlan{
		ID: "subscription-live-smoke", Revision: "r1",
		Routes: []catalog.RouteActivation{{
			RouteID: routeID, Action: catalog.ActivateHostBlockedRoute,
			ExpectedEvidenceDigest: entry.Evidence.Digest, ExpectedAdapterID: entry.Implementation.AdapterID,
		}},
	}
	factories, err := routegateway.NewBuiltinFactoryRegistry()
	if err != nil {
		t.Fatal(err)
	}
	resolver := liveSubscriptionAuthorizationResolver{now: now}
	secret := liveSmokeSecretResolver{now: now, key: key, routeID: entry.ID, identity: entry.Route.Identity(), profile: entry.Route.Credential}
	gateway, report, err := routegateway.NewHost(routegateway.HostConfig{
		BaseCatalog: base, ActivationPlan: &plan,
		BindingResolver: routegateway.CatalogBindingResolver{}, SecretResolver: secret,
		SubscriptionAuthorizationResolver: resolver, Factories: factories,
		Clock: func() time.Time { return now },
	})
	if err != nil || gateway == nil || !report.Ready {
		t.Fatalf("construct subscription Gateway: report=%#v err=%v", report, err)
	}
	defer func() {
		if closeErr := gateway.Close(); closeErr != nil {
			t.Errorf("Gateway.Close() error = %v", closeErr)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := gateway.Invoke(ctx, modelinvoker.RouteCall{
		RouteID: routeID,
		Request: modelinvoker.Request{
			Model:  model,
			Input:  []modelinvoker.InputItem{modelinvoker.MessageInput(modelinvoker.RoleUser, "Reply with exactly: "+subscriptionSmokeMarker)},
			Budget: modelinvoker.Budget{MaxOutputTokens: 32},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasExactSubscriptionSmokeMarker(result.Response.Text()) {
		t.Fatal("subscription Route did not return the exact expected marker")
	}
}

func hasExactSubscriptionSmokeMarker(text string) bool {
	return strings.TrimSpace(text) == subscriptionSmokeMarker
}

func validateSubscriptionSmokeInputs(key string, routeID upstream.RouteID, model string) error {
	if key == "" || routeID == "" || model == "" {
		return fmt.Errorf("key, exact RouteID, and exact model must all be non-empty")
	}
	return nil
}

type liveSubscriptionAuthorizationResolver struct {
	now time.Time
}

func (resolver liveSubscriptionAuthorizationResolver) ResolveSubscriptionAuthorization(_ context.Context, request modelinvoker.SubscriptionAuthorizationRequest) (modelinvoker.SubscriptionAuthorization, error) {
	remaining := int64(1)
	return modelinvoker.SubscriptionAuthorization{
		Invocation: upstream.InvocationContext{
			Explicit: true, Usage: upstream.InvocationInteractiveCoding, Subject: upstream.SubjectPersonal,
			Tenancy: upstream.TenancySingle, Execution: upstream.ExecutionForeground,
			ClientIdentity: upstream.ClientIdentity{
				Name: "praxis-subscription-smoke", Version: "v1.0.0", UserAgent: "praxis-subscription-smoke/v1.0.0",
				Source: upstream.ClientIdentityBuildManifest,
			},
		},
		Entitlement: upstream.EntitlementState{
			OfferingID: request.OfferingID, CredentialProfile: request.CredentialProfile,
			Status: upstream.EntitlementActive, ObservedAt: resolver.now, ValidUntil: resolver.now.Add(5 * time.Minute),
			ExpiresAt: resolver.now.Add(24 * time.Hour), RemainingQuota: &remaining,
		},
	}, nil
}

type liveSmokeSecretResolver struct {
	now      time.Time
	key      string
	routeID  upstream.RouteID
	identity upstream.RouteIdentity
	profile  upstream.CredentialProfile
}

func (resolver liveSmokeSecretResolver) ResolveSecret(_ context.Context, request routegateway.SecretRequest) (routegateway.SecretMaterial, error) {
	if request.RouteID != resolver.routeID || !reflect.DeepEqual(request.Identity, resolver.identity) || !reflect.DeepEqual(request.Profile, resolver.profile) {
		return routegateway.SecretMaterial{}, fmt.Errorf("live smoke secret request does not match its exact Route and Credential Profile")
	}
	return routegateway.NewSecretMaterial(
		request.Profile.ID, request.Profile.Type, "live-smoke-v1", resolver.now.Add(time.Hour),
		map[upstream.CredentialPurpose][]byte{upstream.CredentialPurposeAPIKey: []byte(resolver.key)},
	)
}

func TestLiveSmokeSecretResolverRejectsCrossRouteAndReferenceDrift(t *testing.T) {
	now := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	base, err := catalog.NewDefault(now)
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := base.Get("kimi.code-membership.global.chat_completions")
	resolver := liveSmokeSecretResolver{
		now: now, key: "test-only-subscription-key", routeID: selected.ID,
		identity: selected.Route.Identity(), profile: selected.Route.Clone().Credential,
	}
	selectedRequest := routegateway.SecretRequest{RouteID: selected.ID, Identity: selected.Route.Identity(), Profile: selected.Route.Clone().Credential}
	if _, err := resolver.ResolveSecret(context.Background(), selectedRequest); err != nil {
		t.Fatalf("selected Route secret resolution failed: %v", err)
	}
	other, _ := base.Get("openai.direct.payg.responses")
	if _, err := resolver.ResolveSecret(context.Background(), routegateway.SecretRequest{
		RouteID: other.ID, Identity: other.Route.Identity(), Profile: other.Route.Credential,
	}); err == nil {
		t.Fatal("subscription resolver accepted a cross-Route credential request")
	}
	drifted := selectedRequest
	drifted.Profile = selected.Route.Clone().Credential
	drifted.Profile.References[0].Name = "OTHER_SECRET_REFERENCE"
	if _, err := resolver.ResolveSecret(context.Background(), drifted); err == nil {
		t.Fatal("subscription resolver accepted a drifted Credential Reference")
	}
}

func TestSubscriptionSmokeMarkerIsExact(t *testing.T) {
	for _, value := range []string{"not-empty", "prefix " + subscriptionSmokeMarker, subscriptionSmokeMarker + " suffix", strings.ToUpper(subscriptionSmokeMarker)} {
		if hasExactSubscriptionSmokeMarker(value) {
			t.Fatalf("non-exact marker %q was accepted", value)
		}
	}
	if !hasExactSubscriptionSmokeMarker(" \n" + subscriptionSmokeMarker + "\t") {
		t.Fatal("exact marker with transport whitespace was rejected")
	}
}

func TestSubscriptionSmokeInputGateRequiresEveryExplicitValue(t *testing.T) {
	for _, input := range []struct {
		key     string
		routeID upstream.RouteID
		model   string
	}{
		{routeID: "kimi.code-membership.global.chat_completions", model: "kimi-for-coding"},
		{key: "test-only-key", model: "kimi-for-coding"},
		{key: "test-only-key", routeID: "kimi.code-membership.global.chat_completions"},
	} {
		if err := validateSubscriptionSmokeInputs(input.key, input.routeID, input.model); err == nil {
			t.Fatalf("incomplete live input was accepted: route=%q", input.routeID)
		}
	}
	if err := validateSubscriptionSmokeInputs("test-only-key", "kimi.code-membership.global.chat_completions", "kimi-for-coding"); err != nil {
		t.Fatalf("complete live input was rejected: %v", err)
	}
}
