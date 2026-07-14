package control_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBindingLifecycleRequiresCertifiedGrantAndHonorsTTLBoundary(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_000, 0)
	manifest, catalog := controlBindingFixture(t, "vendor/component", "vendor/kind", nil, nil)
	declared := declaredBindingV2(t, "binding-component", manifest, catalog)
	probed := probedBindingV2(t, declared, now)
	if err := control.ValidateBindingFactTransitionV2(declared, probed, now); err != nil {
		t.Fatal(err)
	}
	certified := certifiedBindingV2(t, probed, now.Add(time.Second))
	if err := control.ValidateBindingFactTransitionV2(probed, certified, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	bound := certified
	bound.State = control.BindingBound
	bound.Revision++
	bound.BindingSetID = "set-1"
	if err := control.ValidateBindingFactTransitionV2(certified, bound, time.Unix(0, certified.ExpiresUnixNano)); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("TTL boundary must be expired, got %v", err)
	}
	withoutGrant := probed
	withoutGrant.Grants = nil
	if err := withoutGrant.Validate(); !core.HasReason(err, core.ReasonUnknownCapability) {
		t.Fatalf("manifest declaration must not become a capability grant: %v", err)
	}
}

func TestBindingLifecycleFailsClosedOnClockRegression(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_500, 0)
	manifest, catalog := controlBindingFixture(t, "vendor/component", "vendor/kind", nil, nil)
	probed := probedBindingV2(t, declaredBindingV2(t, "binding-component", manifest, catalog), now)
	certified := certifiedBindingV2(t, probed, now.Add(time.Second))
	if err := control.ValidateBindingFactTransitionV2(probed, certified, now.Add(-time.Nanosecond)); !core.HasReason(err, core.ReasonClockRegression) {
		t.Fatalf("clock rollback before persisted probe must fail closed: %v", err)
	}
}

func TestBuildBindingSetDoesNotPromoteOptionalDependency(t *testing.T) {
	t.Parallel()
	now := time.Unix(1_700, 0)
	manifest, catalog := controlBindingFixture(t, "vendor/component", "vendor/kind", []ports.ComponentDependencyV2{{ComponentID: "vendor/optional", Optional: true}}, []ports.CapabilityRequirementV2{{Capability: "vendor/optional-capability", ProviderComponent: "vendor/optional", Optional: true}})
	certified := certifiedBindingV2(t, probedBindingV2(t, declaredBindingV2(t, "binding-component", manifest, catalog), now), now.Add(time.Second))
	digest, _ := catalog.DigestV2()
	plan := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "plan-optional", GovernanceDigest: digest, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(manifest)}})
	set, err := control.BuildBindingSetV2("set-optional", plan, catalog, []control.BindingFactV2{certified}, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("missing optional dependency must remain optional: %v", err)
	}
	if len(set.Members) != 1 || set.Members[0].ComponentID != manifest.ComponentID {
		t.Fatalf("optional dependency must not remove or promote the selected component: %+v", set)
	}
}

func TestBuildBindingSetUsesStableDependencyOrderAndOwnerMayRepeatRoles(t *testing.T) {
	t.Parallel()
	now := time.Unix(2_000, 0)
	provider, providerCatalog := controlBindingFixture(t, "vendor/provider", "vendor/provider-kind", nil, nil)
	consumerDependencies := []ports.ComponentDependencyV2{{ComponentID: provider.ComponentID, Optional: false}}
	consumerRequirements := []ports.CapabilityRequirementV2{{Capability: "vendor/execute", ProviderComponent: provider.ComponentID, Optional: false}}
	consumer, consumerCatalog := controlBindingFixture(t, "vendor/consumer", "vendor/consumer-kind", consumerDependencies, consumerRequirements)
	catalog := ports.GovernanceCatalogV2{Registrations: append(providerCatalog.Registrations, consumerCatalog.Registrations...)}
	catalogDigest, err := catalog.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	providerFact := certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-provider", provider, catalogDigest), now), now.Add(time.Second))
	consumerFact := certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-consumer", consumer, catalogDigest), now), now.Add(time.Second))
	plan := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "plan-1", GovernanceDigest: catalogDigest, Requirements: []ports.BindingRequirementV2{
		bindingRequirementV2(consumer), bindingRequirementV2(provider),
	}})
	set, err := control.BuildBindingSetV2("set-1", plan, catalog, []control.BindingFactV2{consumerFact, providerFact}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(set.TopologicalOrder) != 2 || set.TopologicalOrder[0] != provider.ComponentID || set.TopologicalOrder[1] != consumer.ComponentID {
		t.Fatalf("stable dependency order is wrong: %+v", set.TopologicalOrder)
	}
	for _, manifest := range []ports.ComponentManifestV2{provider, consumer} {
		if manifest.Owners[0].OwnerComponentID != manifest.Owners[1].OwnerComponentID {
			t.Fatal("fixture should prove one component may own multiple distinct roles")
		}
	}
}

func TestBuildBindingSetRejectsCapabilityDependencyCycle(t *testing.T) {
	t.Parallel()
	now := time.Unix(3_000, 0)
	aDep := []ports.ComponentDependencyV2{{ComponentID: "vendor/b", Optional: false}}
	aReq := []ports.CapabilityRequirementV2{{Capability: "vendor/execute", ProviderComponent: "vendor/b", Optional: false}}
	bDep := []ports.ComponentDependencyV2{{ComponentID: "vendor/a", Optional: false}}
	bReq := []ports.CapabilityRequirementV2{{Capability: "vendor/execute", ProviderComponent: "vendor/a", Optional: false}}
	a, aCatalog := controlBindingFixture(t, "vendor/a", "vendor/a-kind", aDep, aReq)
	b, bCatalog := controlBindingFixture(t, "vendor/b", "vendor/b-kind", bDep, bReq)
	catalog := ports.GovernanceCatalogV2{Registrations: append(aCatalog.Registrations, bCatalog.Registrations...)}
	digest, _ := catalog.DigestV2()
	facts := []control.BindingFactV2{
		certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-a", a, digest), now), now.Add(time.Second)),
		certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-b", b, digest), now), now.Add(time.Second)),
	}
	plan := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "plan-cycle", GovernanceDigest: digest, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(a), bindingRequirementV2(b)}})
	if _, err := control.BuildBindingSetV2("set-cycle", plan, catalog, facts, now.Add(2*time.Second)); !core.HasReason(err, core.ReasonDependencyCycle) {
		t.Fatalf("component/capability dependency cycle must fail closed: %v", err)
	}
}

func TestValidateBindingSetCurrentRejectsManifestDriftAndMissingProbe(t *testing.T) {
	t.Parallel()
	now := time.Unix(4_000, 0)
	manifest, catalog := controlBindingFixture(t, "vendor/component", "vendor/kind", nil, nil)
	certified := certifiedBindingV2(t, probedBindingV2(t, declaredBindingV2(t, "binding-component", manifest, catalog), now), now.Add(time.Second))
	catalogDigest, _ := catalog.DigestV2()
	plan := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "plan-1", GovernanceDigest: catalogDigest, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(manifest)}})
	set, err := control.BuildBindingSetV2("set-1", plan, catalog, []control.BindingFactV2{certified}, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	bound := certified
	bound.State = control.BindingBound
	bound.Revision++
	bound.BindingSetID = set.ID
	if err := control.ValidateBindingSetCurrentV2(set, []control.BindingFactV2{bound}, nil, now.Add(3*time.Second)); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("missing current probe must invalidate the binding set: %v", err)
	}
	if err := control.ValidateBindingSetCurrentV2(set, []control.BindingFactV2{bound}, []control.BindingCurrentProbeV2{{ComponentID: manifest.ComponentID, ManifestDigest: controlDigestV2(t, "drift")}}, now.Add(3*time.Second)); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("manifest digest drift must invalidate the grant: %v", err)
	}
}

func TestBindingPlanV2CanonicalDigestRejectsSemanticSwapAndUnknownRequiredCapability(t *testing.T) {
	t.Parallel()
	now := time.Unix(4_500, 0)
	first, firstCatalog := controlBindingFixture(t, "vendor/first", "vendor/first-kind", nil, nil)
	second, secondCatalog := controlBindingFixture(t, "vendor/second", "vendor/second-kind", nil, nil)
	catalog := ports.GovernanceCatalogV2{Registrations: append(firstCatalog.Registrations, secondCatalog.Registrations...)}
	governanceDigest, err := catalog.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	plan := sealedBindingPlanV2(t, ports.BindingPlanV2{ID: "plan-canonical", GovernanceDigest: governanceDigest, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(first), bindingRequirementV2(second)}})

	reordered := plan
	reordered.Requirements = []ports.BindingRequirementV2{plan.Requirements[1], plan.Requirements[0]}
	if err := reordered.Validate(); err != nil {
		t.Fatalf("semantic requirement set order changed the Plan identity: %v", err)
	}

	swapped := plan
	swapped.Requirements = append([]ports.BindingRequirementV2{}, plan.Requirements...)
	swapped.Requirements[0].Kind = second.Kind
	swapped.Requirements[0].ArtifactDigest = second.ArtifactDigest
	if err := swapped.Validate(); !core.HasReason(err, core.ReasonPlanInvalid) {
		t.Fatalf("same PlanDigest accepted swapped kind/artifact semantics: %v", err)
	}

	nilCapabilities := ports.BindingPlanV2{ID: "plan-nil-empty", GovernanceDigest: governanceDigest, Requirements: []ports.BindingRequirementV2{bindingRequirementV2(first)}}
	nilCapabilities.Requirements[0].RequiredCapabilities = nil
	emptyCapabilities := nilCapabilities
	emptyCapabilities.Requirements = append([]ports.BindingRequirementV2{}, nilCapabilities.Requirements...)
	emptyCapabilities.Requirements[0].RequiredCapabilities = []ports.CapabilityNameV2{}
	nilDigest, err := ports.BindingPlanDigestV2(nilCapabilities)
	if err != nil {
		t.Fatal(err)
	}
	emptyDigest, err := ports.BindingPlanDigestV2(emptyCapabilities)
	if err != nil || nilDigest != emptyDigest {
		t.Fatalf("nil/empty required capability sets have different Plan identity: nil=%s empty=%s err=%v", nilDigest, emptyDigest, err)
	}

	unknown := plan
	unknown.Requirements = append([]ports.BindingRequirementV2{}, plan.Requirements...)
	unknown.Requirements[0].RequiredCapabilities = []ports.CapabilityNameV2{"vendor/unknown"}
	unknown = sealedBindingPlanV2(t, unknown)
	firstFact := certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-first", first, governanceDigest), now), now.Add(time.Second))
	secondFact := certifiedBindingV2(t, probedBindingV2(t, declaredBindingWithDigestV2(t, "binding-second", second, governanceDigest), now), now.Add(time.Second))
	if _, err := control.BuildBindingSetV2("set-unknown", unknown, catalog, []control.BindingFactV2{firstFact, secondFact}, now.Add(2*time.Second)); !core.HasReason(err, core.ReasonUnknownCapability) {
		t.Fatalf("unknown required capability entered a BindingSet: %v", err)
	}
}

func controlBindingFixture(t *testing.T, id, kind string, dependencies []ports.ComponentDependencyV2, requirements []ports.CapabilityRequirementV2) (ports.ComponentManifestV2, ports.GovernanceCatalogV2) {
	t.Helper()
	manifest := ports.ComponentManifestV2{
		ContractVersion: ports.BindingContractVersionV2, ComponentID: ports.ComponentIDV2(id), Kind: ports.ComponentKindV2(kind), GovernanceCategory: "vendor/execution", SemanticVersion: "1.0.0", ArtifactDigest: controlDigestV2(t, "artifact-"+id),
		Contract: ports.ContractBindingV2{Name: "vendor/execution-contract", Version: "2.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}},
		Schemas:  []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: dependencies, RequiredCapabilities: requirements,
		ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "vendor/execute", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}},
		Conformance:          ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable,
		Owners:      []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: ports.ComponentIDV2(id)}, {Role: ports.OwnerSettlement, OwnerComponentID: ports.ComponentIDV2(id)}, {Role: ports.OwnerCleanup, OwnerComponentID: ports.ComponentIDV2(id)}},
		Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{},
	}
	catalog := ports.GovernanceCatalogV2{Registrations: []ports.GovernanceRegistrationV2{{Kind: manifest.Kind, Category: manifest.GovernanceCategory, Capabilities: []ports.CapabilityNameV2{"vendor/execute"}, Schemas: []ports.SchemaRefV2{}, ExtensionPolicies: []ports.ExtensionPolicyV2{}, AllowedLocalities: []ports.LocalityV2{ports.LocalityHostControlPlane}, AllowedConformance: []ports.ConformanceLevel{ports.ConformanceFullyControlled}}}}
	return manifest, catalog
}

func declaredBindingV2(t *testing.T, id string, manifest ports.ComponentManifestV2, catalog ports.GovernanceCatalogV2) control.BindingFactV2 {
	t.Helper()
	digest, err := catalog.DigestV2()
	if err != nil {
		t.Fatal(err)
	}
	return declaredBindingWithDigestV2(t, id, manifest, digest)
}

func declaredBindingWithDigestV2(t *testing.T, id string, manifest ports.ComponentManifestV2, governanceDigest core.Digest) control.BindingFactV2 {
	t.Helper()
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	fact := control.BindingFactV2{ID: id, ComponentID: manifest.ComponentID, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governanceDigest, State: control.BindingDeclared, Revision: 1, Grants: []ports.CapabilityGrantV2{}}
	if err := fact.Validate(); err != nil {
		t.Fatal(err)
	}
	return fact
}

func probedBindingV2(t *testing.T, declared control.BindingFactV2, now time.Time) control.BindingFactV2 {
	t.Helper()
	probed := declared
	probed.State = control.BindingProbed
	probed.Revision++
	probed.ProbedUnixNano = now.UnixNano()
	probed.ExpiresUnixNano = now.Add(5 * time.Minute).UnixNano()
	probed.Grants = make([]ports.CapabilityGrantV2, 0, len(probed.Manifest.ProvidedCapabilities))
	for _, capability := range probed.Manifest.ProvidedCapabilities {
		probed.Grants = append(probed.Grants, ports.CapabilityGrantV2{Capability: capability.Capability, EvidenceDigest: controlDigestV2(t, string(capability.Capability)), ObservedUnixNano: now.UnixNano(), ExpiresUnixNano: probed.ExpiresUnixNano})
	}
	return probed
}

func certifiedBindingV2(t *testing.T, probed control.BindingFactV2, now time.Time) control.BindingFactV2 {
	t.Helper()
	certified := probed
	certified.State = control.BindingCertified
	certified.Revision++
	certified.CertifiedUnixNano = now.UnixNano()
	certified.ConformanceEvidenceDigest = controlDigestV2(t, "conformance-"+certified.ID)
	return certified
}

func bindingRequirementV2(manifest ports.ComponentManifestV2) ports.BindingRequirementV2 {
	capabilities := make([]ports.CapabilityNameV2, 0, len(manifest.ProvidedCapabilities))
	for _, capability := range manifest.ProvidedCapabilities {
		capabilities = append(capabilities, capability.Capability)
	}
	return ports.BindingRequirementV2{ComponentID: manifest.ComponentID, Kind: manifest.Kind, SemanticVersion: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}, ContractName: manifest.Contract.Name, Contract: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}, ArtifactDigest: manifest.ArtifactDigest, RequiredCapabilities: capabilities, Required: true}
}

func sealedBindingPlanV2(t *testing.T, plan ports.BindingPlanV2) ports.BindingPlanV2 {
	t.Helper()
	sealed, err := ports.SealBindingPlanV2(plan)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}

func controlDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}
