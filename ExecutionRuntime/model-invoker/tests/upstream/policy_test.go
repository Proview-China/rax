package upstream_test

import (
	"reflect"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/model-invoker/upstream"
)

func interactiveContext() upstream.InvocationContext {
	return upstream.InvocationContext{
		Explicit:   true,
		Usage:      upstream.InvocationInteractiveCoding,
		Subject:    upstream.SubjectPersonal,
		Tenancy:    upstream.TenancySingle,
		Execution:  upstream.ExecutionForeground,
		Production: false,
		ClientIdentity: upstream.ClientIdentity{
			Name:      "praxis",
			Version:   "1.0.0",
			UserAgent: "praxis/1.0.0",
			Source:    upstream.ClientIdentityRuntimeObserved,
		},
	}
}

func TestGeneralAPIEntitlementAllowsServiceProductionContexts(t *testing.T) {
	t.Parallel()
	entitlement := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI}
	context := upstream.InvocationContext{
		Explicit:   true,
		Usage:      upstream.InvocationGeneralAPI,
		Subject:    upstream.SubjectService,
		Tenancy:    upstream.TenancyMulti,
		Execution:  upstream.ExecutionBatch,
		Production: true,
	}
	decision := entitlement.Decide(context)
	if !decision.Allowed || decision.Code != upstream.PolicyAllowed || len(decision.Reasons) != 0 {
		t.Fatalf("Decide() = %#v, want allowed", decision)
	}
}

func TestProvidedClientIdentityMustAlwaysBeAttested(t *testing.T) {
	t.Parallel()
	entitlement := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI}
	context := upstream.InvocationContext{
		Explicit:   true,
		Usage:      upstream.InvocationGeneralAPI,
		Subject:    upstream.SubjectService,
		Tenancy:    upstream.TenancySingle,
		Execution:  upstream.ExecutionForeground,
		Production: true,
		ClientIdentity: upstream.ClientIdentity{
			Name:      "claimed-client",
			Version:   "1.0",
			UserAgent: "claimed-client/1.0",
			Source:    "user_supplied",
		},
	}
	decision := entitlement.Decide(context)
	if decision.Allowed || !decisionContains(decision, upstream.PolicyInvalidClientIdentity) {
		t.Fatalf("Decide() = %#v, want invalid identity denial", decision)
	}
}

func TestInteractiveCodingDefaultGate(t *testing.T) {
	t.Parallel()
	entitlement := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageInteractiveCodingOnly, RequiresExplicitContext: true}
	valid := interactiveContext()
	if decision := entitlement.Decide(valid); !decision.Allowed {
		t.Fatalf("valid interactive context denied: %#v", decision)
	}
	tests := []struct {
		name   string
		code   upstream.PolicyReasonCode
		mutate func(*upstream.InvocationContext)
	}{
		{name: "not explicit", code: upstream.PolicyExplicitContextRequired, mutate: func(context *upstream.InvocationContext) { context.Explicit = false }},
		{name: "general backend", code: upstream.PolicyUsageNotAllowed, mutate: func(context *upstream.InvocationContext) { context.Usage = upstream.InvocationGeneralAPI }},
		{name: "service subject", code: upstream.PolicyPersonalSubjectRequired, mutate: func(context *upstream.InvocationContext) { context.Subject = upstream.SubjectService }},
		{name: "multi tenant", code: upstream.PolicySingleTenantRequired, mutate: func(context *upstream.InvocationContext) { context.Tenancy = upstream.TenancyMulti }},
		{name: "batch", code: upstream.PolicyForegroundRequired, mutate: func(context *upstream.InvocationContext) { context.Execution = upstream.ExecutionBatch }},
		{name: "background", code: upstream.PolicyForegroundRequired, mutate: func(context *upstream.InvocationContext) { context.Execution = upstream.ExecutionBackground }},
		{name: "production", code: upstream.PolicyProductionForbidden, mutate: func(context *upstream.InvocationContext) { context.Production = true }},
		{name: "missing identity", code: upstream.PolicyClientIdentityRequired, mutate: func(context *upstream.InvocationContext) { context.ClientIdentity = upstream.ClientIdentity{} }},
		{name: "unattested identity", code: upstream.PolicyInvalidClientIdentity, mutate: func(context *upstream.InvocationContext) { context.ClientIdentity.Source = "user_supplied" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context := valid
			test.mutate(&context)
			decision := entitlement.Decide(context)
			if decision.Allowed || !decisionContains(decision, test.code) {
				t.Fatalf("Decide() = %#v, want denial %q", decision, test.code)
			}
		})
	}
}

func TestInteractiveCodingAlwaysRequiresExplicitContext(t *testing.T) {
	t.Parallel()
	entitlement := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageInteractiveCodingOnly}
	context := interactiveContext()
	context.Explicit = false
	decision := entitlement.Decide(context)
	if decision.Allowed || !decisionContains(decision, upstream.PolicyExplicitContextRequired) {
		t.Fatalf("Decide() = %#v, want explicit context denial", decision)
	}
}

func TestOfficialClientOnlyAndClientAllowlistAreHardGates(t *testing.T) {
	t.Parallel()
	context := interactiveContext()
	official := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageOfficialClientOnly}
	if decision := official.Decide(context); decision.Allowed || !decisionContains(decision, upstream.PolicyOfficialClientOnly) {
		t.Fatalf("official-only Decide() = %#v", decision)
	}
	restricted := upstream.CommercialEntitlement{
		AllowedUsage:           upstream.AllowedUsageGeneralAPI,
		RequiresClientIdentity: true,
		AllowedClientNames:     []string{"approved-client"},
	}
	if decision := restricted.Decide(context); decision.Allowed || !decisionContains(decision, upstream.PolicyClientIdentityNotAllowed) {
		t.Fatalf("client allowlist Decide() = %#v", decision)
	}
}

func TestPolicyDecisionIsStableAndCloneSafe(t *testing.T) {
	t.Parallel()
	entitlement := upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageInteractiveCodingOnly, RequiresExplicitContext: true}
	context := interactiveContext()
	context.Explicit = false
	context.Subject = upstream.SubjectService
	context.Tenancy = upstream.TenancyMulti
	first := entitlement.Decide(context)
	second := entitlement.Decide(context)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Decide() is not deterministic:\n%#v\n%#v", first, second)
	}
	clone := first.Clone()
	clone.Reasons[0].Message = "mutated"
	if first.Reasons[0].Message == "mutated" {
		t.Fatal("PolicyDecision.Clone() retained reasons slice")
	}
}

func TestAllowsCompatibilityEntryPointIsPreserved(t *testing.T) {
	t.Parallel()
	general := upstream.Offering{Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageGeneralAPI}}
	coding := upstream.Offering{Entitlement: upstream.CommercialEntitlement{AllowedUsage: upstream.AllowedUsageInteractiveCodingOnly}}
	if !general.Allows(upstream.InvocationGeneralAPI) || !general.Allows(upstream.InvocationInteractiveCoding) {
		t.Fatal("general Allows compatibility changed")
	}
	if coding.Allows(upstream.InvocationGeneralAPI) || !coding.Allows(upstream.InvocationInteractiveCoding) {
		t.Fatal("coding Allows compatibility changed")
	}
}

func decisionContains(decision upstream.PolicyDecision, code upstream.PolicyReasonCode) bool {
	for _, reason := range decision.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
