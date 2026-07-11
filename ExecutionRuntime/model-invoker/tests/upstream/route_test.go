package upstream_test

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func validRoute() upstream.UpstreamRoute {
	return upstream.UpstreamRoute{
		ID:       "vendor.direct.payg.responses",
		Model:    upstream.ModelIdentity{CanonicalFamily: "vendor-model", ProviderModelRef: "runtime_selected"},
		Provider: "vendor",
		Offering: upstream.Offering{
			ID:   "vendor.api.payg",
			Kind: upstream.OfferingPayAsYouGo,
			Entitlement: upstream.CommercialEntitlement{
				AllowedUsage:           upstream.AllowedUsageGeneralAPI,
				ClientRestrictions:     []string{"honest-client-identity"},
				SubjectPolicy:          upstream.SubjectPolicyAny,
				TenancyPolicy:          upstream.TenancyPolicyAny,
				ExecutionPolicy:        upstream.ExecutionPolicyAny,
				ProductionPolicy:       upstream.ProductionPolicyAllowed,
				RequiresClientIdentity: true,
				AllowedClientNames:     []string{"praxis"},
			},
		},
		Deployment: upstream.Deployment{ID: "vendor.direct.global", Kind: upstream.DeploymentDirect, Region: "global"},
		Protocol:   upstream.ProtocolBinding{ID: upstream.ProtocolResponses, APIVersion: "v1"},
		Endpoint: upstream.Endpoint{
			ID:                 "vendor.public",
			Scheme:             "https",
			HostTemplate:       "api.vendor.example",
			BasePath:           "/v1",
			CredentialAudience: "api.vendor.example",
		},
		Credential: upstream.CredentialProfile{
			ID:                   "vendor.default",
			Type:                 upstream.CredentialAPIKey,
			References:           []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "VENDOR_API_KEY"}},
			Audience:             "api.vendor.example",
			AuthPlacement:        upstream.AuthPlacementHeader,
			AuthHeader:           "Authorization",
			AuthScheme:           "Bearer",
			Scopes:               []string{"models.invoke"},
			KeyPrefixes:          []string{"vk-"},
			AllowedProviderIDs:   []upstream.ProviderID{"vendor"},
			AllowedOfferingIDs:   []upstream.OfferingID{"vendor.api.payg"},
			AllowedDeploymentIDs: []upstream.DeploymentID{"vendor.direct.global"},
			AllowedRegions:       []string{"global"},
			AllowedEndpointIDs:   []upstream.EndpointID{"vendor.public"},
			Lifecycle:            upstream.CredentialLifecycleStatic,
		},
	}
}

func TestRouteRequiresAllSevenDimensions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.UpstreamRoute)
	}{
		{name: "model", field: "model", mutate: func(route *upstream.UpstreamRoute) { route.Model = upstream.ModelIdentity{} }},
		{name: "provider", field: "provider", mutate: func(route *upstream.UpstreamRoute) { route.Provider = "" }},
		{name: "offering", field: "offering", mutate: func(route *upstream.UpstreamRoute) { route.Offering = upstream.Offering{} }},
		{name: "deployment", field: "deployment", mutate: func(route *upstream.UpstreamRoute) { route.Deployment = upstream.Deployment{} }},
		{name: "protocol", field: "protocol", mutate: func(route *upstream.UpstreamRoute) { route.Protocol = upstream.ProtocolBinding{} }},
		{name: "endpoint", field: "endpoint", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint = upstream.Endpoint{} }},
		{name: "credential", field: "credential", mutate: func(route *upstream.UpstreamRoute) { route.Credential = upstream.CredentialProfile{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			route := validRoute()
			test.mutate(&route)
			var validationError *upstream.ValidationError
			if err := route.Validate(); !errors.As(err, &validationError) || !validationError.HasField(test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestRouteValidationPolicyAndCredentialBinding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		field  string
		mutate func(*upstream.UpstreamRoute)
	}{
		{name: "invalid usage", field: "offering.entitlement.allowed_usage", mutate: func(route *upstream.UpstreamRoute) { route.Offering.Entitlement.AllowedUsage = "serverish" }},
		{name: "implicit payg fallback", field: "offering.entitlement.allows_automatic_payg_switch", mutate: func(route *upstream.UpstreamRoute) { route.Offering.Entitlement.AllowsAutomaticPAYGSwitch = true }},
		{name: "credential audience", field: "credential.audience", mutate: func(route *upstream.UpstreamRoute) { route.Credential.Audience = "other.example" }},
		{name: "endpoint allowlist", field: "credential.allowed_endpoint_ids", mutate: func(route *upstream.UpstreamRoute) {
			route.Credential.AllowedEndpointIDs = []upstream.EndpointID{"other.public"}
		}},
		{name: "plaintext-like reference", field: "credential.references", mutate: func(route *upstream.UpstreamRoute) { route.Credential.References[0].Name = "sk value with spaces" }},
		{name: "insecure public endpoint", field: "endpoint.scheme", mutate: func(route *upstream.UpstreamRoute) { route.Endpoint.Scheme = "http" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			route := validRoute()
			test.mutate(&route)
			var validationError *upstream.ValidationError
			if err := route.Validate(); !errors.As(err, &validationError) || !validationError.HasField(test.field) {
				t.Fatalf("Validate() error = %v, want field %q", err, test.field)
			}
		})
	}
}

func TestOfferingAllowedUsage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		policy  upstream.AllowedUsage
		usage   upstream.InvocationUsage
		allowed bool
	}{
		{name: "general backend", policy: upstream.AllowedUsageGeneralAPI, usage: upstream.InvocationGeneralAPI, allowed: true},
		{name: "general interactive", policy: upstream.AllowedUsageGeneralAPI, usage: upstream.InvocationInteractiveCoding, allowed: true},
		{name: "coding rejects backend", policy: upstream.AllowedUsageInteractiveCodingOnly, usage: upstream.InvocationGeneralAPI, allowed: false},
		{name: "coding accepts interactive", policy: upstream.AllowedUsageInteractiveCodingOnly, usage: upstream.InvocationInteractiveCoding, allowed: true},
		{name: "official client blocked", policy: upstream.AllowedUsageOfficialClientOnly, usage: upstream.InvocationInteractiveCoding, allowed: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			offering := upstream.Offering{Entitlement: upstream.CommercialEntitlement{AllowedUsage: test.policy}}
			if got := offering.Allows(test.usage); got != test.allowed {
				t.Fatalf("Allows(%q) = %v, want %v", test.usage, got, test.allowed)
			}
		})
	}
}

func TestCredentialProfileSchemaContainsReferencesNotValues(t *testing.T) {
	t.Parallel()
	encoded, err := json.Marshal(validRoute().Credential)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	for _, forbidden := range []string{"value", "token", "secret", "api_key"} {
		if _, exists := fields[forbidden]; exists {
			t.Fatalf("credential schema contains plaintext field %q: %s", forbidden, encoded)
		}
	}
	if _, exists := fields["references"]; !exists {
		t.Fatalf("credential schema lost references: %s", encoded)
	}
}

func TestRouteAndMappingReportCloneAreConcurrentCopySafe(t *testing.T) {
	route := validRoute()
	report := upstream.MappingReport{
		Identity:       route.Identity(),
		RouteID:        route.ID,
		Provider:       route.Provider,
		EvidenceDigest: "sha256:597ec867f0351b62c60aadd2f32c240d94a546876da196a26502741d48a1cb8c",
		Reasons:        []upstream.MappingReason{{Code: "exact"}},
		CapabilityDecisions: []upstream.CapabilityDecision{{
			Capability: "text_generation",
			Action:     upstream.CapabilityExact,
			ReasonCode: "native",
		}},
		Degradations: []string{"fixture"},
	}
	const workers = 64
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			clone := route.Clone()
			clone.Offering.Entitlement.ClientRestrictions[0] = "changed"
			clone.Credential.References[0].Name = "CHANGED"
			clone.Credential.AllowedEndpointIDs[0] = "changed"
			clone.Credential.Scopes[0] = "changed"
			clone.Credential.AllowedProviderIDs[0] = "changed"
			clone.Offering.Entitlement.AllowedClientNames[0] = "changed"
			if err := clone.Validate(); err == nil {
				t.Error("mutated clone unexpectedly remained valid")
			}
			reportClone := report.Clone()
			reportClone.Reasons[0].Code = "changed"
			reportClone.CapabilityDecisions[0].ReasonCode = "changed"
			reportClone.Degradations[0] = "changed"
		}()
	}
	wait.Wait()
	if route.Credential.References[0].Name != "VENDOR_API_KEY" || route.Credential.AllowedEndpointIDs[0] != "vendor.public" || route.Credential.Scopes[0] != "models.invoke" || route.Credential.AllowedProviderIDs[0] != "vendor" || route.Offering.Entitlement.AllowedClientNames[0] != "praxis" {
		t.Fatalf("route clone mutated source: %#v", route.Credential)
	}
	if report.Reasons[0].Code != "exact" || report.CapabilityDecisions[0].ReasonCode != "native" || report.Degradations[0] != "fixture" {
		t.Fatalf("report clone mutated source: %#v", report)
	}
}
