package kernel_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/kernel"
)

func TestConformancePlacementMatrix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		mutate     func(*contract.ExecutionRequirement, *contract.PolicyProjection, *contract.BackendDescriptor, *contract.PlacementCandidate)
		admitted   bool
		wantReason string
	}{
		{name: "admitted", admitted: true},
		{name: "observed only", mutate: func(_ *contract.ExecutionRequirement, _ *contract.PolicyProjection, b *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			b.Capabilities[contract.CapabilityProcessFence] = contract.CapabilityObservedOnly
		}, wantReason: "not enforced"},
		{name: "raw bypass high risk", mutate: func(_ *contract.ExecutionRequirement, _ *contract.PolicyProjection, b *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			b.RawBypass = true
		}, wantReason: "raw bypass"},
		{name: "remote without remote controls", mutate: func(_ *contract.ExecutionRequirement, _ *contract.PolicyProjection, b *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			b.Surface = contract.SurfaceRemoteSandbox
			b.Locality = contract.LocalityRemoteProvider
			b.Capabilities[contract.CapabilityCleanupCoverage] = contract.CapabilityObservedOnly
		}, wantReason: "remote backend"},
		{name: "remote exact controls admitted", mutate: func(r *contract.ExecutionRequirement, _ *contract.PolicyProjection, b *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			r.AllowedSurfaces = []contract.ExecutionSurface{contract.SurfaceRemoteSandbox}
			b.Surface = contract.SurfaceRemoteSandbox
			b.Locality = contract.LocalityRemoteProvider
		}, admitted: true},
		{name: "effects enabled", mutate: func(_ *contract.ExecutionRequirement, p *contract.PolicyProjection, _ *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			p.ExternalEffectsDisabled = false
		}, wantReason: "external_effects_disabled"},
		{name: "policy expands write scope", mutate: func(_ *contract.ExecutionRequirement, p *contract.PolicyProjection, _ *contract.BackendDescriptor, _ *contract.PlacementCandidate) {
			p.WriteScopes = []string{"outside"}
		}, wantReason: "projection-error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement, policy, backend, candidate := testkit.Requirement(), testkit.Policy(), testkit.Backend(), testkit.Candidate()
			if test.mutate != nil {
				test.mutate(&requirement, &policy, &backend, &candidate)
			}
			decision, err := kernel.EvaluatePlacement(testkit.FixedNow, requirement, policy, backend, candidate)
			if test.wantReason == "projection-error" {
				if err == nil || !strings.Contains(err.Error(), "expands requirement") {
					t.Fatalf("projection error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if decision.Admitted != test.admitted {
				t.Fatalf("Admitted = %v, reasons = %v", decision.Admitted, decision.Reasons)
			}
			if test.wantReason != "" && !strings.Contains(strings.Join(decision.Reasons, " "), test.wantReason) {
				t.Fatalf("reasons = %v, want %q", decision.Reasons, test.wantReason)
			}
		})
	}
}

func TestNoGoPlacementRejectsAuthorityScopeBindingAndTTLDrift(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		mutate func(*contract.PolicyProjection, *contract.BackendDescriptor)
	}{
		{name: "authority policy revision", mutate: func(p *contract.PolicyProjection, _ *contract.BackendDescriptor) {
			p.AuthorityRef = testkit.Ref("authority-drift")
			p.Meta = testkit.Meta("policy", 2)
		}},
		{name: "scope policy revision", mutate: func(p *contract.PolicyProjection, _ *contract.BackendDescriptor) {
			p.ScopeDigest = testkit.Ref("scope-drift").Digest
			p.Meta = testkit.Meta("policy", 2)
		}},
		{name: "backend binding revision", mutate: func(_ *contract.PolicyProjection, b *contract.BackendDescriptor) {
			b.Meta = testkit.Meta("backend", 2)
		}},
		{name: "policy ttl", mutate: func(p *contract.PolicyProjection, _ *contract.BackendDescriptor) {
			p.Meta.CreatedUnixNano = testkit.FixedNow.Add(-2 * time.Hour).UnixNano()
			p.Meta.UpdatedUnixNano = testkit.FixedNow.Add(-time.Hour).UnixNano()
			p.Meta.ExpiresUnixNano = testkit.FixedNow.UnixNano()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirement, policy, backend, candidate := testkit.Requirement(), testkit.Policy(), testkit.Backend(), testkit.Candidate()
			test.mutate(&policy, &backend)
			decision, err := kernel.EvaluatePlacement(testkit.FixedNow, requirement, policy, backend, candidate)
			if err == nil || decision.Admitted {
				t.Fatalf("drift was admitted: decision=%#v err=%v", decision, err)
			}
		})
	}
}
