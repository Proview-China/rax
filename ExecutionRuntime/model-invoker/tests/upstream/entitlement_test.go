package upstream_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func TestSubscriptionAuthorizeRequiresFreshBoundActiveState(t *testing.T) {
	now := time.Date(2026, time.July, 11, 0, 45, 0, 0, time.UTC)
	route := subscriptionRoute()
	quota := int64(100)
	state := upstream.EntitlementState{
		OfferingID: route.Offering.ID, CredentialProfile: route.Credential.ID,
		Status: upstream.EntitlementActive, ObservedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Minute),
		ExpiresAt: now.Add(24 * time.Hour), RemainingQuota: &quota, QuotaResetAt: now.Add(5 * time.Hour),
	}
	invocation := interactiveInvocation()
	decision := route.Authorize(invocation, &state, now)
	if !decision.Allowed || decision.Code != upstream.PolicyAllowed || decision.AllowsAutomaticPAYGSwitch {
		t.Fatalf("Authorize(valid) = %#v", decision)
	}

	tests := []struct {
		name   string
		want   upstream.PolicyReasonCode
		mutate func(*upstream.EntitlementState)
	}{
		{name: "missing state", want: upstream.PolicyEntitlementStateRequired},
		{name: "wrong offering", want: upstream.PolicyEntitlementBinding, mutate: func(s *upstream.EntitlementState) { s.OfferingID = "other.plan" }},
		{name: "wrong credential", want: upstream.PolicyEntitlementBinding, mutate: func(s *upstream.EntitlementState) { s.CredentialProfile = "other.key" }},
		{name: "stale", want: upstream.PolicyEntitlementStateStale, mutate: func(s *upstream.EntitlementState) { s.ValidUntil = now }},
		{name: "expired timestamp", want: upstream.PolicyEntitlementExpired, mutate: func(s *upstream.EntitlementState) { s.ExpiresAt = now }},
		{name: "expired status", want: upstream.PolicyEntitlementExpired, mutate: func(s *upstream.EntitlementState) { s.Status = upstream.EntitlementExpired }},
		{name: "zero quota", want: upstream.PolicyEntitlementQuota, mutate: func(s *upstream.EntitlementState) { zero := int64(0); s.RemainingQuota = &zero }},
		{name: "quota status", want: upstream.PolicyEntitlementQuota, mutate: func(s *upstream.EntitlementState) { s.Status = upstream.EntitlementQuotaExhausted }},
		{name: "suspended", want: upstream.PolicyEntitlementSuspended, mutate: func(s *upstream.EntitlementState) { s.Status = upstream.EntitlementSuspended }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := state
			if test.mutate != nil {
				test.mutate(&candidate)
			}
			var candidateState *upstream.EntitlementState
			if test.name != "missing state" {
				candidateState = &candidate
			}
			decision := route.Authorize(invocation, candidateState, now)
			if decision.Allowed || decision.Code != test.want || decision.AllowsAutomaticPAYGSwitch {
				t.Fatalf("Authorize() = %#v, want %q and no fallback", decision, test.want)
			}
		})
	}
}

func TestSubscriptionAuthorizeKeepsStaticInteractiveBoundaries(t *testing.T) {
	route := subscriptionRoute()
	now := time.Date(2026, time.July, 11, 0, 45, 0, 0, time.UTC)
	quota := int64(1)
	state := upstream.EntitlementState{
		OfferingID: route.Offering.ID, CredentialProfile: route.Credential.ID,
		Status: upstream.EntitlementActive, ObservedAt: now.Add(-time.Minute), ValidUntil: now.Add(time.Minute), RemainingQuota: &quota,
	}
	tests := []struct {
		name   string
		want   upstream.PolicyReasonCode
		mutate func(*upstream.InvocationContext)
	}{
		{name: "general api", want: upstream.PolicyUsageNotAllowed, mutate: func(c *upstream.InvocationContext) { c.Usage = upstream.InvocationGeneralAPI }},
		{name: "implicit", want: upstream.PolicyExplicitContextRequired, mutate: func(c *upstream.InvocationContext) { c.Explicit = false }},
		{name: "service", want: upstream.PolicyPersonalSubjectRequired, mutate: func(c *upstream.InvocationContext) { c.Subject = upstream.SubjectService }},
		{name: "multi tenant", want: upstream.PolicySingleTenantRequired, mutate: func(c *upstream.InvocationContext) { c.Tenancy = upstream.TenancyMulti }},
		{name: "background", want: upstream.PolicyForegroundRequired, mutate: func(c *upstream.InvocationContext) { c.Execution = upstream.ExecutionBackground }},
		{name: "production", want: upstream.PolicyProductionForbidden, mutate: func(c *upstream.InvocationContext) { c.Production = true }},
		{name: "missing identity", want: upstream.PolicyClientIdentityRequired, mutate: func(c *upstream.InvocationContext) { c.ClientIdentity = upstream.ClientIdentity{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invocation := interactiveInvocation()
			test.mutate(&invocation)
			decision := route.Authorize(invocation, &state, now)
			if decision.Allowed || decision.Code != test.want || decision.AllowsAutomaticPAYGSwitch {
				t.Fatalf("Authorize() = %#v, want %q", decision, test.want)
			}
		})
	}
}

func TestSubscriptionHTTPFailuresNeverSwitchToPAYG(t *testing.T) {
	route := subscriptionRoute()
	for status, want := range map[int]upstream.PolicyReasonCode{
		401: upstream.PolicyEntitlementCredential,
		402: upstream.PolicyEntitlementBilling,
		403: upstream.PolicyEntitlementAccessDenied,
		429: upstream.PolicyEntitlementQuota,
	} {
		decision := route.DenyHTTPFailure(status)
		if decision.Allowed || decision.Code != want || decision.AllowsAutomaticPAYGSwitch {
			t.Errorf("DenyHTTPFailure(%d) = %#v, want %q and no fallback", status, decision, want)
		}
	}
}

func TestCredentialResolvedSecretPrefixIsFailClosedAndNonDisclosing(t *testing.T) {
	profile := subscriptionRoute().Credential
	profile.KeyPrefixes = []string{"sk-cp-", "tp-"}
	for _, valid := range []string{"sk-cp-example", "tp-example"} {
		if err := profile.ValidateResolvedSecret(valid); err != nil {
			t.Fatalf("ValidateResolvedSecret(valid) error = %v", err)
		}
	}
	for _, invalid := range []string{"", "sk-payg-secret", " tp-secret", "tp-secret\nleak"} {
		err := profile.ValidateResolvedSecret(invalid)
		var typed *upstream.CredentialSecretError
		if !errors.As(err, &typed) || (invalid != "" && strings.Contains(err.Error(), invalid)) {
			t.Fatalf("ValidateResolvedSecret(%q) = %#v (%v)", invalid, typed, err)
		}
	}
}

func TestCredentialResolvedSecretDeniedPrefixTakesPrecedence(t *testing.T) {
	profile := subscriptionRoute().Credential
	profile.KeyPrefixes = []string{"sk-"}
	profile.DeniedKeyPrefixes = []string{"sk-sp-"}
	for _, valid := range []string{"sk-ws-payg", "sk-legacy-payg"} {
		if err := profile.ValidateResolvedSecret(valid); err != nil {
			t.Fatalf("ValidateResolvedSecret(%q) error = %v", valid, err)
		}
	}
	if err := profile.ValidateResolvedSecret("sk-sp-subscription"); err == nil {
		t.Fatal("denied subscription prefix accepted through broader allow prefix")
	}
}

func TestSavingsPlanIsBillingMetadataNotRouteIdentity(t *testing.T) {
	route := subscriptionRoute()
	route.Offering.Kind = upstream.OfferingPayAsYouGo
	route.Offering.Entitlement.AllowedUsage = upstream.AllowedUsageGeneralAPI
	route.Offering.BillingPlan = &upstream.BillingPlanReference{
		ID: "vendor.savings", Kind: upstream.BillingPlanSavings,
		BillingOwner: "Vendor Billing", AppliesToOfferingID: route.Offering.ID,
	}
	before := route.Identity()
	if err := route.Validate(); err != nil {
		t.Fatalf("Validate(savings plan) error = %v", err)
	}
	after := route.Identity()
	if before != after {
		t.Fatalf("billing plan changed route identity: %#v != %#v", before, after)
	}
	clone := route.Clone()
	clone.Offering.BillingPlan.ID = "mutated"
	if route.Offering.BillingPlan.ID == "mutated" {
		t.Fatal("Clone retained caller-owned billing-plan pointer")
	}

	invalid := route
	invalid.Offering.BillingPlan = &upstream.BillingPlanReference{
		ID: "vendor.savings", Kind: upstream.BillingPlanSavings,
		BillingOwner: "Vendor Billing", AppliesToOfferingID: "other.payg",
	}
	if err := invalid.Validate(); err == nil {
		t.Fatal("Validate accepted a billing plan bound to another offering")
	}
}

func interactiveInvocation() upstream.InvocationContext {
	return upstream.InvocationContext{
		Explicit: true, Usage: upstream.InvocationInteractiveCoding,
		Subject: upstream.SubjectPersonal, Tenancy: upstream.TenancySingle,
		Execution: upstream.ExecutionForeground,
		ClientIdentity: upstream.ClientIdentity{
			Name: "praxis-cli", Version: "v1.0.0", UserAgent: "praxis-cli/v1.0.0", Source: upstream.ClientIdentityBuildManifest,
		},
	}
}

func subscriptionRoute() upstream.UpstreamRoute {
	return upstream.UpstreamRoute{
		ID:       "vendor.coding.chat_completions",
		Model:    upstream.ModelIdentity{CanonicalFamily: "vendor", ProviderModelRef: "coding-model"},
		Provider: "vendor",
		Offering: upstream.Offering{
			ID: "vendor.coding", Kind: upstream.OfferingCodingPlan,
			Entitlement: upstream.CommercialEntitlement{
				AllowedUsage:            upstream.AllowedUsageInteractiveCodingOnly,
				RequiresExplicitContext: true, SubjectPolicy: upstream.SubjectPolicyPersonalOnly,
				TenancyPolicy: upstream.TenancyPolicySingleTenantOnly, ExecutionPolicy: upstream.ExecutionPolicyForegroundOnly,
				ProductionPolicy: upstream.ProductionPolicyForbidden, RequiresClientIdentity: true,
			},
		},
		Deployment: upstream.Deployment{ID: "vendor.direct.global", Kind: upstream.DeploymentDirect, Region: "global"},
		Protocol:   upstream.ProtocolBinding{ID: upstream.ProtocolChatCompletions},
		Endpoint:   upstream.Endpoint{ID: "vendor.coding", Scheme: "https", HostTemplate: "coding.vendor.test", BasePath: "/v1", CredentialAudience: "coding.vendor.test"},
		Credential: upstream.CredentialProfile{
			ID: "vendor.coding", Type: upstream.CredentialAPIKey,
			References: []upstream.CredentialReference{{Purpose: upstream.CredentialPurposeAPIKey, Store: "env", Name: "VENDOR_CODING_API_KEY"}},
			Audience:   "coding.vendor.test", AuthPlacement: upstream.AuthPlacementHeader, AuthHeader: "Authorization", AuthScheme: "Bearer",
			AllowedProviderIDs: []upstream.ProviderID{"vendor"}, AllowedOfferingIDs: []upstream.OfferingID{"vendor.coding"},
			AllowedDeploymentIDs: []upstream.DeploymentID{"vendor.direct.global"}, AllowedRegions: []string{"global"},
			AllowedEndpointIDs: []upstream.EndpointID{"vendor.coding"}, Lifecycle: upstream.CredentialLifecycleStatic,
		},
	}
}

func FuzzCredentialResolvedSecretNeverLeaks(f *testing.F) {
	f.Add("sk-cp-valid")
	f.Add("secret\nline")
	f.Add("")
	profile := subscriptionRoute().Credential
	profile.KeyPrefixes = []string{"sk-cp-"}
	f.Fuzz(func(t *testing.T, value string) {
		candidate := "praxis-secret-sentinel-begin-" + value + "-praxis-secret-sentinel-end"
		err := profile.ValidateResolvedSecret(candidate)
		if err != nil && strings.Contains(err.Error(), candidate) {
			t.Fatal("credential validation error leaked input")
		}
	})
}
