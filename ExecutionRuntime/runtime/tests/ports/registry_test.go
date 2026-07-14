package ports_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type staticDescriber struct{ descriptor ports.ComponentDescriptor }

func (s staticDescriber) Describe(context.Context) (ports.ComponentDescriptor, error) {
	return s.descriptor, nil
}

type changingDescriber struct {
	descriptors []ports.ComponentDescriptor
	index       int
}

func (c *changingDescriber) Describe(context.Context) (ports.ComponentDescriptor, error) {
	index := c.index
	if index >= len(c.descriptors) {
		index = len(c.descriptors) - 1
	}
	c.index++
	return c.descriptors[index], nil
}

func TestRegistryResolvesBoundComponentsAndOptionalResiduals(t *testing.T) {
	t.Parallel()
	now := time.Now()
	harness := registryDescriptor(t, "harness", ports.ComponentHarness, now.Add(time.Hour))
	registry := ports.NewComponentRegistry()
	if err := registry.Register(context.Background(), staticDescriber{harness}); err != nil {
		t.Fatal(err)
	}
	plan := registryPlan(t, harness, true)
	missingDigest := registryDigest(t, "optional")
	plan.Requirements = append(plan.Requirements, ports.ComponentRequirement{ID: "optional-context", Kind: ports.ComponentContextEngine, Version: "v1", ArtifactDigest: missingDigest, AllowResidual: true})
	bindings, err := registry.Resolve(context.Background(), plan, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Descriptors) != 1 || len(bindings.Residuals) != 1 || bindings.Residuals[0].ComponentID != "optional-context" {
		t.Fatalf("unexpected bindings: %+v", bindings)
	}
}

func TestRegistryRejectsExpiredBoundCapability(t *testing.T) {
	t.Parallel()
	now := time.Now()
	harness := registryDescriptor(t, "harness", ports.ComponentHarness, now.Add(-time.Second))
	registry := ports.NewComponentRegistry()
	if err := registry.Register(context.Background(), staticDescriber{harness}); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Resolve(context.Background(), registryPlan(t, harness, true), now)
	if !core.HasReason(err, core.ReasonCapabilityExpired) {
		t.Fatalf("expired capability evidence must fail: %v", err)
	}
}

func TestRegistryRejectsAdapterWhoseIdentityDriftsAfterRegistration(t *testing.T) {
	t.Parallel()
	now := time.Now()
	original := registryDescriptor(t, "harness", ports.ComponentHarness, now.Add(time.Hour))
	drifted := original
	drifted.ID = "different-harness"
	adapter := &changingDescriber{descriptors: []ports.ComponentDescriptor{original, drifted}}
	registry := ports.NewComponentRegistry()
	if err := registry.Register(context.Background(), adapter); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve(context.Background(), registryPlan(t, original, true), now); !core.HasReason(err, core.ReasonComponentMismatch) {
		t.Fatalf("adapter identity drift must fail binding: %v", err)
	}
}

func TestResolvedPlanRejectsDependencyCycle(t *testing.T) {
	t.Parallel()
	a := registryDescriptor(t, "a", ports.ComponentHarness, time.Now().Add(time.Hour))
	b := registryDescriptor(t, "b", ports.ComponentSandbox, time.Now().Add(time.Hour))
	plan := registryPlan(t, a, false)
	plan.Requirements = []ports.ComponentRequirement{
		{ID: "a", Kind: a.Kind, Version: a.Version, ArtifactDigest: a.ArtifactDigest, Required: true, DependsOn: []string{"b"}},
		{ID: "b", Kind: b.Kind, Version: b.Version, ArtifactDigest: b.ArtifactDigest, Required: true, DependsOn: []string{"a"}},
	}
	if err := plan.Validate(); !core.HasReason(err, core.ReasonDependencyCycle) {
		t.Fatalf("cyclic component graph must fail: %v", err)
	}
}

func TestRegistryRejectsRequiredComponentWhoseDependencyBecameResidual(t *testing.T) {
	t.Parallel()
	now := time.Now()
	harness := registryDescriptor(t, "harness", ports.ComponentHarness, now.Add(time.Hour))
	registry := ports.NewComponentRegistry()
	if err := registry.Register(context.Background(), staticDescriber{harness}); err != nil {
		t.Fatal(err)
	}
	plan := registryPlan(t, harness, true)
	plan.Requirements[0].DependsOn = []string{"optional-context"}
	plan.Requirements = append(plan.Requirements, ports.ComponentRequirement{
		ID: "optional-context", Kind: ports.ComponentContextEngine, Version: "v1",
		ArtifactDigest: registryDigest(t, "optional"), AllowResidual: true,
	})
	if _, err := registry.Resolve(context.Background(), plan, now); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("required component cannot bind without its dependency: %v", err)
	}
}

func TestRegistryCascadesOptionalDependencyResiduals(t *testing.T) {
	t.Parallel()
	now := time.Now()
	harness := registryDescriptor(t, "optional-harness", ports.ComponentHarness, now.Add(time.Hour))
	registry := ports.NewComponentRegistry()
	if err := registry.Register(context.Background(), staticDescriber{harness}); err != nil {
		t.Fatal(err)
	}
	plan := registryPlan(t, harness, false)
	plan.Requirements[0].Required = false
	plan.Requirements[0].AllowResidual = true
	plan.Requirements[0].DependsOn = []string{"optional-context"}
	plan.Requirements = append(plan.Requirements, ports.ComponentRequirement{
		ID: "optional-context", Kind: ports.ComponentContextEngine, Version: "v1",
		ArtifactDigest: registryDigest(t, "optional"), AllowResidual: true,
	})
	bindings, err := registry.Resolve(context.Background(), plan, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings.Descriptors) != 0 || len(bindings.Residuals) != 2 {
		t.Fatalf("optional dependency chain must become residual: %+v", bindings)
	}
}

func registryDescriptor(t *testing.T, id string, kind ports.ComponentKind, expiry time.Time) ports.ComponentDescriptor {
	t.Helper()
	digest := registryDigest(t, id)
	return ports.ComponentDescriptor{ID: id, Kind: kind, Version: "v1", ArtifactDigest: digest, ContractVersion: ports.ContractVersion, Conformance: ports.ConformanceFullyControlled, Capabilities: []ports.Capability{{Name: "run", State: ports.CapabilityBound, EvidenceDigest: digest, EvidenceExpiry: expiry}}}
}

func registryPlan(t *testing.T, descriptor ports.ComponentDescriptor, requireRun bool) ports.ResolvedAgentPlan {
	t.Helper()
	capabilities := []string(nil)
	if requireRun {
		capabilities = []string{"run"}
	}
	return ports.ResolvedAgentPlan{ID: "plan-1", Digest: registryDigest(t, "plan"), ProfileDigest: registryDigest(t, "profile"), ContextDigest: registryDigest(t, "context"), AuthorityDigest: registryDigest(t, "authority"), Requirements: []ports.ComponentRequirement{{ID: descriptor.ID, Kind: descriptor.Kind, Version: descriptor.Version, ArtifactDigest: descriptor.ArtifactDigest, Required: true, Capabilities: capabilities}}}
}

func registryDigest(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
