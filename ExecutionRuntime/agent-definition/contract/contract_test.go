package contract_test

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/conformance"
	"github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestSealValidateCloneAndCanonicalSetOrderingV1(t *testing.T) {
	now := time.Unix(1_800_000_000, 123)
	source := conformance.SourceV1(now)
	catalog := conformance.CatalogV1()
	first, err := contract.SealDefinitionV1(source, catalog, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Validate(catalog); err != nil {
		t.Fatal(err)
	}
	reversed := contract.CloneSourceV1(source)
	slices.Reverse(reversed.Components)
	for index := range reversed.Components {
		slices.Reverse(reversed.Components[index].RequiredCapabilities)
	}
	second, err := contract.SealDefinitionV1(reversed, catalog, now.UnixNano())
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest != second.Digest || first.SourceDigest != second.SourceDigest {
		t.Fatalf("set order changed canonical digest: %s != %s", first.Digest, second.Digest)
	}
	nilCollections := contract.CloneSourceV1(source)
	nilCollections.SecretRefs = nil
	nilCollections.Extensions = nil
	third, err := contract.SealDefinitionV1(nilCollections, catalog, now.UnixNano())
	if err != nil || third.Digest != first.Digest {
		t.Fatalf("nil/empty policy drift: %s %s %v", first.Digest, third.Digest, err)
	}
	clone := contract.CloneDefinitionV1(first)
	clone.Components[0].ComponentID = "agent/mutated"
	clone.Components[1].RequiredCapabilities[0] = "attacker/capability"
	if len(clone.Extensions) == 0 {
		clone.Extensions = []contract.ExtensionV1{{Payload: json.RawMessage(`{"safe":true}`)}}
	}
	if first.Components[0].ComponentID == clone.Components[0].ComponentID {
		t.Fatal("clone aliases original")
	}
	if first.Components[1].RequiredCapabilities[0] == clone.Components[1].RequiredCapabilities[0] {
		t.Fatal("clone aliases nested component slices")
	}
	payload, err := json.Marshal(first)
	if err != nil || !json.Valid(payload) {
		t.Fatalf("strict JSON output: %v", err)
	}
}

func TestCustomOptionalComponentUsesCatalogWithoutKindSwitchV1(t *testing.T) {
	now := time.Unix(1_800_000_050, 0)
	source := conformance.SourceV1(now)
	catalog := conformance.CatalogV1()
	catalog.Kinds = append(catalog.Kinds, "custom/vector-engine")
	catalog.Capabilities = append(catalog.Capabilities, "custom/vector-query")
	source.Components = append(source.Components, contract.ComponentRequirementV1{
		ComponentID: "custom/vector-primary", Kind: "custom/vector-engine",
		SemanticVersion: contract.VersionRangeV1{MinimumInclusive: "1.2.0", MaximumExclusive: "2.0.0"},
		ContractName:    "custom/vector-contract", ContractVersion: contract.VersionRangeV1{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"},
		RequiredCapabilities: []string{"custom/vector-query"}, Required: false, SupportMode: contract.SupportModeProductionV1,
		LocalityConstraint: contract.LocalityExternalStatePlaneV1, DependencyIDs: []string{},
	})
	definition, err := contract.SealDefinitionV1(source, catalog, now.UnixNano())
	if err != nil || len(definition.Components) != 8 {
		t.Fatalf("custom component: %#v %v", definition.Components, err)
	}
}

func TestRequiredCoreKindsAccessorReturnsIsolatedCopyV1(t *testing.T) {
	first := contract.RequiredCoreKindsV1()
	if len(first) == 0 {
		t.Fatal("required core kinds empty")
	}
	original := first[0]
	first[0] = "attacker/mutated"
	second := contract.RequiredCoreKindsV1()
	if second[0] != original {
		t.Fatalf("exported core kind state was mutable: %q", second[0])
	}
}

func TestDefinitionValidationFailClosedMatrixV1(t *testing.T) {
	now := time.Unix(1_800_000_100, 0)
	catalog := conformance.CatalogV1()
	tests := []struct {
		name   string
		mutate func(*contract.AgentDefinitionSourceV1)
		reason core.ReasonCode
	}{
		{"missing-core", func(s *contract.AgentDefinitionSourceV1) { s.Components = s.Components[1:] }, core.ReasonComponentMissing},
		{"non-production", func(s *contract.AgentDefinitionSourceV1) { s.Components[0].SupportMode = "standalone" }, core.ReasonComponentMismatch},
		{"unknown-kind", func(s *contract.AgentDefinitionSourceV1) { s.Components[0].Kind = "custom/unknown" }, core.ReasonUnknownGovernanceCategory},
		{"unknown-capability", func(s *contract.AgentDefinitionSourceV1) {
			s.Components[0].RequiredCapabilities = []string{"custom/unknown"}
		}, core.ReasonUnknownCapability},
		{"absolute-secret", func(s *contract.AgentDefinitionSourceV1) {
			s.SecretRefs = []contract.SecretRefV1{{SecretID: "/tmp/secret", Class: "secret/token", RequestedScopeDigest: conformance.DigestV1("scope")}}
		}, core.ReasonInvalidReference},
		{"dependency-cycle", func(s *contract.AgentDefinitionSourceV1) {
			s.Components[0].DependencyIDs = []string{s.Components[1].ComponentID}
			s.Components[1].DependencyIDs = []string{s.Components[0].ComponentID}
		}, core.ReasonDependencyCycle},
		{"residual-owner-missing", func(s *contract.AgentDefinitionSourceV1) { s.Components[0].ResidualPolicy.Allowed = true }, core.ReasonOwnerMissing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := conformance.SourceV1(now)
			test.mutate(&source)
			_, err := contract.SealDefinitionV1(source, catalog, now.UnixNano())
			if !core.HasReason(err, test.reason) {
				t.Fatalf("error = %v, want %s", err, test.reason)
			}
		})
	}
}

func TestExtensionDigestRequiredRegistrationAndSecretSafetyV1(t *testing.T) {
	now := time.Unix(1_800_000_200, 0)
	source := conformance.SourceV1(now)
	payload := json.RawMessage(`{"mode":"safe"}`)
	source.Extensions = []contract.ExtensionV1{{Key: "example/required", Required: true, Schema: contract.SchemaRefV1{Namespace: "example", Name: "extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: conformance.DigestV1("schema")}, ContentDigest: core.DigestBytes(payload), Payload: payload}}
	if _, err := contract.SealDefinitionV1(source, conformance.CatalogV1(), now.UnixNano()); err != nil {
		t.Fatal(err)
	}
	source.Extensions[0].Key = "unknown/required"
	if _, err := contract.SealDefinitionV1(source, conformance.CatalogV1(), now.UnixNano()); !core.HasReason(err, core.ReasonUnknownRequiredExtension) {
		t.Fatalf("unknown required extension = %v", err)
	}
	source.Extensions[0].Required = false
	if _, err := contract.SealDefinitionV1(source, conformance.CatalogV1(), now.UnixNano()); err != nil {
		t.Fatalf("unknown optional extension was not preserved: %v", err)
	}
	source.Extensions[0].Required = true
	source.Extensions[0].Key = "example/required"
	bad := json.RawMessage(`{"password":"plaintext"}`)
	source.Extensions[0].Payload = bad
	source.Extensions[0].ContentDigest = core.DigestBytes(bad)
	if _, err := contract.SealDefinitionV1(source, conformance.CatalogV1(), now.UnixNano()); !core.HasReason(err, core.ReasonInvalidReference) {
		t.Fatalf("secret extension = %v", err)
	}
}

func TestExtensionSecretKeyAndStringValueMatrixFailClosedV1(t *testing.T) {
	now := time.Unix(1_800_000_225, 0)
	catalog := conformance.CatalogV1()
	tests := map[string]any{
		"secret": "value", "token": "value", "password": "value", "private_key": "value", "api_key": "value",
		"client-secret": "value", "refresh_token": "value", "x-api-key": "value",
		"authorization": "value", "credential-cache": "value", "session-cookie": "value",
		"safe-sk": "sk-live-secret", "safe-bearer": "Bearer abc", "safe-pem": "-----BEGIN PRIVATE KEY-----",
		"safe-file": "file:///tmp/key", "safe-absolute": "/tmp/key", "safe-traversal": "../../key",
		"safe-windows": `C:\\Users\\agent\\key`, "safe-unc": `\\\\server\\share\\key`,
	}
	for name, item := range tests {
		t.Run(name, func(t *testing.T) {
			payload, err := json.Marshal(map[string]any{name: item})
			if err != nil {
				t.Fatal(err)
			}
			source := conformance.SourceV1(now)
			source.Extensions = []contract.ExtensionV1{{Key: "example/required", Required: true, Schema: contract.SchemaRefV1{Namespace: "example", Name: "extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: conformance.DigestV1("schema")}, ContentDigest: core.DigestBytes(payload), Payload: payload}}
			if _, err := contract.SealDefinitionV1(source, catalog, now.UnixNano()); !core.HasReason(err, core.ReasonInvalidReference) {
				t.Fatalf("forbidden extension content accepted: %s error=%v", payload, err)
			}
		})
	}
}

func TestExtensionSafetyAllowsNormalRelativeIdentifiersV1(t *testing.T) {
	now := time.Unix(1_800_000_227, 0)
	payload := json.RawMessage(`{"mode":"safe","resource_id":"models/checkpoint-v1","route":"namespace/component"}`)
	source := conformance.SourceV1(now)
	source.Extensions = []contract.ExtensionV1{{Key: "example/required", Required: true, Schema: contract.SchemaRefV1{Namespace: "example", Name: "extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: conformance.DigestV1("schema")}, ContentDigest: core.DigestBytes(payload), Payload: payload}}
	if _, err := contract.SealDefinitionV1(source, conformance.CatalogV1(), now.UnixNano()); err != nil {
		t.Fatalf("normal relative identifiers were rejected: %v", err)
	}
}

func TestValidationCatalogCloneIsDeepAndSingleRegistrationSetV1(t *testing.T) {
	catalog := conformance.CatalogV1()
	clone := contract.CloneValidationCatalogV1(catalog)
	clone.Kinds[0] = "attacker/kind"
	clone.Capabilities[0] = "attacker/capability"
	clone.RegisteredExtensionKeys[0] = "attacker/extension"
	if catalog.Kinds[0] == clone.Kinds[0] || catalog.Capabilities[0] == clone.Capabilities[0] || catalog.RegisteredExtensionKeys[0] == clone.RegisteredExtensionKeys[0] {
		t.Fatal("validation catalog clone aliases caller slices")
	}
	payload := json.RawMessage(`{"mode":"safe"}`)
	source := conformance.SourceV1(time.Unix(1_800_000_230, 0))
	source.Extensions = []contract.ExtensionV1{{Key: "example/required", Required: true, Payload: payload}}
	sourceClone := contract.CloneSourceV1(source)
	sourceClone.Extensions[0].Payload[0] = '['
	if source.Extensions[0].Payload[0] != '{' {
		t.Fatal("source clone aliases extension payload bytes")
	}
}

func TestExtensionDuplicateJSONKeyAndSecretPathTraversalRejectedV1(t *testing.T) {
	now := time.Unix(1_800_000_250, 0)
	catalog := conformance.CatalogV1()
	source := conformance.SourceV1(now)
	duplicate := json.RawMessage(`{"mode":"safe","mode":"changed"}`)
	source.Extensions = []contract.ExtensionV1{{Key: "example/required", Required: true, Schema: contract.SchemaRefV1{Namespace: "example", Name: "extension", Version: "1.0.0", MediaType: "application/json", ContentDigest: conformance.DigestV1("schema")}, ContentDigest: core.DigestBytes(duplicate), Payload: duplicate}}
	if _, err := contract.SealDefinitionV1(source, catalog, now.UnixNano()); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate extension key = %v", err)
	}
	for _, secretID := range []string{"../../x", "secret/../x", "file://tmp/secret", "/tmp/secret"} {
		candidate := conformance.SourceV1(now)
		candidate.SecretRefs = []contract.SecretRefV1{{SecretID: secretID, Class: "secret/token", RequestedScopeDigest: conformance.DigestV1("scope")}}
		if _, err := contract.SealDefinitionV1(candidate, catalog, now.UnixNano()); err == nil {
			t.Fatalf("secret path accepted: %q", secretID)
		}
	}
}

func FuzzSourceCanonicalOrderingV1(f *testing.F) {
	f.Add(uint8(0))
	f.Add(uint8(1))
	f.Fuzz(func(t *testing.T, shift uint8) {
		now := time.Unix(1_800_001_000, 0)
		source := conformance.SourceV1(now)
		baseline, err := contract.SourceDigestV1(source, conformance.CatalogV1())
		if err != nil {
			t.Fatal(err)
		}
		if len(source.Components) > 0 {
			amount := int(shift) % len(source.Components)
			source.Components = append(source.Components[amount:], source.Components[:amount]...)
		}
		actual, err := contract.SourceDigestV1(source, conformance.CatalogV1())
		if err != nil || actual != baseline {
			t.Fatalf("canonical drift %s %s %v", baseline, actual, err)
		}
	})
}
