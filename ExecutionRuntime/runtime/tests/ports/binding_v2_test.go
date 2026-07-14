package ports_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type staticDescriberV2 struct{ manifest ports.ComponentManifestV2 }

func (s staticDescriberV2) DescribeV2(context.Context) (ports.ComponentManifestV2, error) {
	return s.manifest, nil
}

func TestManifestV2CanonicalDigestNormalizesSetsAndExcludesAnnotations(t *testing.T) {
	t.Parallel()
	manifest, catalog := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	manifest.Dependencies = []ports.ComponentDependencyV2{{ComponentID: "vendor/z", Optional: true}, {ComponentID: "vendor/a", Optional: true}}
	left, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	manifest.Dependencies[0], manifest.Dependencies[1] = manifest.Dependencies[1], manifest.Dependencies[0]
	manifest.Annotations = []ports.DisplayAnnotationV2{{Key: "title", Value: "a different display label"}}
	right, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if left != right {
		t.Fatal("set order and display-only annotations must not change binding identity")
	}
	if err := ports.ValidateManifestAgainstCatalogV2(manifest, catalog); err != nil {
		t.Fatal(err)
	}
}

func TestManifestV2GovernanceExtensionChangesDigestAndRoundTrips(t *testing.T) {
	t.Parallel()
	manifest, catalog := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	before, _ := manifest.BindingDigestV2()
	inline := []byte("opaque governance metadata")
	manifest.Extensions = []ports.GovernanceExtensionV2{{Key: "vendor/policy", Required: false, Payload: ports.OpaquePayloadV2{
		Schema: bindingSchemaV2(t), ContentDigest: core.DigestBytes(inline), Length: uint64(len(inline)), Inline: inline,
		LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "praxis.runtime/default-limit", Digest: bindingDigestV2(t, "limit")},
	}}}
	after, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("governance extension must participate in binding identity")
	}
	if err := ports.ValidateManifestAgainstCatalogV2(manifest, catalog); err != nil {
		t.Fatalf("unknown optional extension must be preserved and accepted: %v", err)
	}
	payload, err := ports.EncodeComponentManifestV2(manifest)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := ports.DecodeComponentManifestV2(payload)
	if err != nil {
		t.Fatal(err)
	}
	roundTripDigest, _ := roundTrip.BindingDigestV2()
	if roundTripDigest != after || len(roundTrip.Extensions) != 1 {
		t.Fatal("unknown optional extension must survive read-write without re-signing drift")
	}
	manifest.Extensions[0].Required = true
	if err := ports.ValidateManifestAgainstCatalogV2(manifest, catalog); !core.HasReason(err, core.ReasonUnknownRequiredExtension) {
		t.Fatalf("unknown required extension must fail closed: %v", err)
	}
}

func TestManifestV2BuildMetadataParticipatesInBindingIdentity(t *testing.T) {
	t.Parallel()
	manifest, _ := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	linuxDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	manifest.SemanticVersion = "1.2.3+windows.amd64"
	windowsDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	if linuxDigest == windowsDigest {
		t.Fatal("component build metadata is part of binding identity")
	}
	rangeV2 := ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}
	if !rangeV2.Contains("1.2.3+linux.amd64") || !rangeV2.Contains("1.2.3+windows.amd64") {
		t.Fatal("SemVer range precedence must ignore build metadata while identity preserves it")
	}
}

func TestManifestV2RejectsUnicodeUppercaseAndUnknownCoreFields(t *testing.T) {
	t.Parallel()
	for _, value := range []ports.NamespacedNameV2{"Vendor/component", "vendor/组件", "vendor//component", "vendor/component/extra"} {
		if err := ports.ValidateNamespacedNameV2(value); !core.HasReason(err, core.ReasonInvalidNamespace) {
			t.Fatalf("non-canonical name %q must fail: %v", value, err)
		}
	}
	manifest, _ := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	payload = append(payload[:len(payload)-1], []byte(`,"unknown_optional":true}`)...)
	if _, err := ports.DecodeComponentManifestV2(payload); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("undeclared top-level field must fail; optional data belongs in extensions: %v", err)
	}
}

func TestRegistryV2RegistrationIsOnlyProbeObservation(t *testing.T) {
	t.Parallel()
	manifest, catalog := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	registry, err := ports.NewComponentRegistryV2(catalog)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := registry.Register(context.Background(), staticDescriberV2{manifest: manifest})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ManifestDigest == "" || observation.Validate() != nil {
		t.Fatal("registration may expose a validated digest but must not fabricate probe timing or grants")
	}
	probed, err := registry.Probe(context.Background(), manifest.ComponentID, time.Unix(100, 0))
	if err != nil || probed.ObservedAt.IsZero() {
		t.Fatalf("explicit probe should be observable but non-authoritative: %v", err)
	}
	zeroProbe := probed
	zeroProbe.ObservedAt = time.Time{}
	if err := zeroProbe.Validate(); !core.HasReason(err, core.ReasonEvidenceUnavailable) {
		t.Fatalf("registration data without probe time must not become probe evidence: %v", err)
	}
}

func TestV1AdapterCannotEscalateFullyControlled(t *testing.T) {
	t.Parallel()
	digest := bindingDigestV2(t, "legacy")
	descriptor := ports.ComponentDescriptor{ID: "legacy-component", Kind: ports.ComponentHarness, Version: "v1", ArtifactDigest: digest, ContractVersion: ports.ContractVersion, Conformance: ports.ConformanceFullyControlled, Capabilities: []ports.Capability{{Name: "run", State: ports.CapabilityDeclared}}}
	manifest, err := ports.AdaptV1DescriptorToManifestV2(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Conformance != ports.ConformanceRestrictedControlled || manifest.ResidualClass != ports.ResidualPotentiallyStale {
		t.Fatal("v1 compatibility adapter must remain restricted and explicitly residual")
	}
}

func TestOpaquePayloadV2RejectsDigestAndLengthDrift(t *testing.T) {
	t.Parallel()
	manifest, _ := bindingV2Fixture(t, "vendor/component", "vendor/kind")
	payload := []byte("body")
	opaque := ports.OpaquePayloadV2{Schema: bindingSchemaV2(t), ContentDigest: core.DigestBytes(payload), Length: uint64(len(payload)), Inline: payload, LimitPolicy: ports.OpaqueLimitPolicyRefV2{Policy: "praxis.runtime/default-limit", Digest: bindingDigestV2(t, "limit")}}
	if err := opaque.Validate(); err != nil {
		t.Fatal(err)
	}
	opaque.Length++
	if err := opaque.Validate(); !core.HasReason(err, core.ReasonCanonicalLimitExceeded) {
		t.Fatalf("opaque length drift must fail: %v", err)
	}
	_ = manifest
}

func bindingV2Fixture(t *testing.T, id string, kind string) (ports.ComponentManifestV2, ports.GovernanceCatalogV2) {
	t.Helper()
	schema := bindingSchemaV2(t)
	manifest := ports.ComponentManifestV2{
		ContractVersion: ports.BindingContractVersionV2,
		ComponentID:     ports.ComponentIDV2(id), Kind: ports.ComponentKindV2(kind), GovernanceCategory: "vendor/execution",
		SemanticVersion: "1.2.3+linux.amd64", ArtifactDigest: bindingDigestV2(t, "artifact-"+id),
		Contract: ports.ContractBindingV2{Name: "vendor/execution-contract", Version: "2.1.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "2.0.0", MaximumExclusive: "3.0.0"}},
		Schemas:  []ports.SchemaRefV2{schema}, Locality: ports.LocalityHostControlPlane,
		Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{},
		ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: "vendor/execute", TTLSeconds: 300, Schemas: []ports.SchemaRefV2{schema}}},
		Conformance:          ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable,
		Owners:      []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: ports.ComponentIDV2(id)}, {Role: ports.OwnerSettlement, OwnerComponentID: ports.ComponentIDV2(id)}, {Role: ports.OwnerCleanup, OwnerComponentID: ports.ComponentIDV2(id)}},
		Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{},
	}
	catalog := ports.GovernanceCatalogV2{Registrations: []ports.GovernanceRegistrationV2{{Kind: ports.ComponentKindV2(kind), Category: "vendor/execution", Capabilities: []ports.CapabilityNameV2{"vendor/execute"}, Schemas: []ports.SchemaRefV2{schema}, ExtensionPolicies: []ports.ExtensionPolicyV2{}, AllowedLocalities: []ports.LocalityV2{ports.LocalityHostControlPlane}, AllowedConformance: []ports.ConformanceLevel{ports.ConformanceFullyControlled, ports.ConformanceRestrictedControlled, ports.ConformanceContainedObserveOnly, ports.ConformanceRejected}}}}
	return manifest, catalog
}

func bindingSchemaV2(t *testing.T) ports.SchemaRefV2 {
	t.Helper()
	return ports.SchemaRefV2{Namespace: "vendor", Name: "request", Version: "1.0.0", MediaType: "application/octet-stream", ContentDigest: bindingDigestV2(t, "schema")}
}

func bindingDigestV2(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func FuzzNamespacedNameV2NeverPanics(f *testing.F) {
	f.Add("vendor/component")
	f.Add("")
	f.Add("vendor/组件")
	f.Fuzz(func(t *testing.T, value string) {
		_ = ports.ValidateNamespacedNameV2(ports.NamespacedNameV2(value))
	})
}
