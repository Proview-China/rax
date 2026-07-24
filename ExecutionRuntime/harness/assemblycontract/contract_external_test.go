package assemblycontract_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	assemblytestkit "github.com/Proview-China/rax/ExecutionRuntime/harness/tests/assembly/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestPublicCatalogAndCanonicalInput(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	for _, slot := range input.Slots {
		if err := slot.Validate(); err != nil {
			t.Fatalf("slot %s: %v", slot.SlotID, err)
		}
	}
	for _, hook := range input.HookFaces {
		if err := hook.Validate(); err != nil {
			t.Fatalf("hook %s: %v", hook.HookFaceID, err)
		}
	}
	reordered := input
	reordered.Modules = slices.Clone(input.Modules)
	slices.Reverse(reordered.Modules)
	reordered.Capabilities = slices.Clone(input.Capabilities)
	slices.Reverse(reordered.Capabilities)
	reordered.Slots = slices.Clone(input.Slots)
	slices.Reverse(reordered.Slots)
	reordered.HookFaces = slices.Clone(input.HookFaces)
	slices.Reverse(reordered.HookFaces)
	reordered.SlotContributions = slices.Clone(input.SlotContributions)
	slices.Reverse(reordered.SlotContributions)
	resealed, err := assemblycontract.SealAssemblyInputV1(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if resealed.Digest != input.Digest {
		t.Fatalf("input digest changed after set reordering: %s != %s", resealed.Digest, input.Digest)
	}
}

func TestInputAndDeclaredDigestTamperingIsVisible(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	input.Slots[0].Digest = assemblytestkit.Digest("forged-slot")
	if err := input.Validate(); !core.HasReason(err, core.ReasonInvalidDigest) {
		t.Fatalf("forged nested digest not rejected: %v", err)
	}
	actual, err := assemblycontract.SlotSpecDigestV1(input.Slots[0])
	if err != nil {
		t.Fatal(err)
	}
	if actual == input.Slots[0].Digest {
		t.Fatal("forged slot digest matched")
	}
	input.Digest = core.Digest("bad")
	if err := input.Validate(); err == nil {
		t.Fatal("malformed input digest accepted")
	}
}

func TestBindingConformanceIsReadOnlyAndCurrent(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	now := assemblytestkit.Now.UnixNano()
	value := assemblycontract.AssemblyBindingConformanceV1{HandoffRef: assemblytestkit.Ref("handoff"), GenerationRef: assemblytestkit.Ref("generation"), ManifestDigest: input.Digest, GraphDigest: assemblytestkit.Digest("graph"), Binding: assemblytestkit.RuntimeBindingRef(), BindingSetDigest: assemblytestkit.Digest("binding-set"), BindingSetSemanticDigest: assemblytestkit.Digest("binding-set-semantic"), CapabilityDigest: assemblytestkit.Digest("capability"), SchemaDigests: []core.Digest{assemblytestkit.Digest("schema-b"), assemblytestkit.Digest("schema-a")}, ObservedUnixNano: now, ExpiresUnixNano: now + 1_000_000, Current: true}
	sealed, err := assemblycontract.SealBindingConformanceV1(value, now+1)
	if err != nil {
		t.Fatal(err)
	}
	if sealed.Digest == "" || sealed.SchemaDigests[0] > sealed.SchemaDigests[1] {
		t.Fatal("binding conformance was not sealed canonically")
	}
	stale := sealed
	stale.ExpiresUnixNano = now
	if err := stale.Validate(now + 1); !core.HasReason(err, core.ReasonBindingExpired) {
		t.Fatalf("expected binding_expired, got %v", err)
	}
}

func TestModuleFactoryDescriptorRejectsRuntimeLoadingModes(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	factory := input.Factories[0]
	factory.ConstructionMode = "remote"
	if err := factory.Validate(); !core.HasReason(err, core.ReasonUnknownCapability) {
		t.Fatalf("remote factory mode accepted in Wave 1: %v", err)
	}
}

func TestLegacyGenericRunRequirementFieldIsRejected(t *testing.T) {
	t.Parallel()
	port := assemblytestkit.ValidInput().PortSpecs[0]
	payload, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(payload, &legacy); err != nil {
		t.Fatal(err)
	}
	legacy["run_requirement_ref"] = map[string]any{"id": "legacy", "revision": 1, "digest": string(assemblytestkit.Digest("legacy"))}
	payload, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assemblycontract.DecodePortSpecV1(payload); !core.HasReason(err, core.ReasonInvalidCanonicalForm) {
		t.Fatalf("legacy generic RunRequirementRef was not rejected: %v", err)
	}
	clean, err := json.Marshal(port)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assemblycontract.DecodePortSpecV1(clean); err != nil {
		t.Fatalf("current PortSpec failed strict decode: %v", err)
	}
}

func TestUniversalFilterWriteSetsAreRejected(t *testing.T) {
	t.Parallel()
	input := assemblytestkit.ValidInput()
	var hook assemblycontract.HookFaceSpecV1
	for _, value := range input.HookFaces {
		if value.Kind == assemblycontract.PhaseFilterV1 {
			hook = value
			break
		}
	}
	for _, path := range []string{"candidate.declared-write-set", "candidate.*"} {
		value := assemblycontract.PhaseContributionV1{ContractVersion: assemblycontract.ContractVersionV1, ContributionID: "praxis.fixture/universal-filter", HookFaceRef: hook.HookFaceID, HandlerDescriptorRef: assemblytestkit.Ref("universal-handler"), ModuleRef: input.Modules[0].ModuleID, Capability: assemblycontract.PhaseFilterV1, WriteSet: []string{path}}
		digest, err := assemblycontract.PhaseContributionDigestV1(value)
		if err != nil {
			t.Fatal(err)
		}
		value.Digest = digest
		if err := value.Validate(); !core.HasReason(err, core.ReasonPlanInvalid) {
			t.Fatalf("universal Filter path %q accepted: %v", path, err)
		}
	}
	for _, hook := range assemblycontract.HookFaceCatalogV1() {
		for _, path := range hook.MutationMask {
			if path == "candidate.declared-write-set" || path == "candidate.*" {
				t.Fatalf("public HookFace Catalog contains universal mutation path %q", path)
			}
		}
	}
}
