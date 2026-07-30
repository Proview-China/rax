package contract

import (
	"reflect"
	"strings"
	"testing"

	assemblercontract "github.com/Proview-China/rax/ExecutionRuntime/agent-assembler/contract"
	definitioncontract "github.com/Proview-China/rax/ExecutionRuntime/agent-definition/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/harness/assemblycontract"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func digest(value string) core.Digest { return core.DigestBytes([]byte(value)) }
func objectRef(id string) assemblycontract.ObjectRefV1 {
	return assemblycontract.ObjectRefV1{ID: id, Revision: 1, Digest: digest(id)}
}

func validLock(t *testing.T) AgentPackageLockManifestV1 {
	t.Helper()
	generation := objectRef("assembly-generation-test")
	inputDigest := digest("input")
	publicationID, err := assemblycontract.DeriveAssemblyPublicationIDV2(inputDigest, generation.ID)
	if err != nil {
		t.Fatal(err)
	}
	value, err := SealLockManifestV1(AgentPackageLockManifestV1{
		DefinitionRef:        definitioncontract.AgentDefinitionRefV1{DefinitionID: "agent/test", Revision: 1, Digest: digest("definition")},
		ResolvedPlanRef:      assemblercontract.ResolvedAgentPlanRefV1{PlanID: "plan/test", Revision: 1, Digest: digest("plan")},
		ResolutionFactsRef:   assemblercontract.ResolutionFactsRefV1{FactsID: "facts/test", Revision: 1, Digest: digest("facts")},
		CatalogRef:           assemblercontract.ComponentReleaseCatalogRefV1{CatalogID: "catalog/test", Revision: 1, Digest: digest("catalog")},
		ComponentReleaseRefs: []assemblercontract.ComponentReleaseRefV1{{ReleaseID: "release/test", Revision: 1, Digest: digest("release"), ComponentID: runtimeports.ComponentIDV2("component/test")}},
		BindingPlanDigest:    digest("binding"), AssemblyInputDigest: inputDigest, FrozenUnixNano: 100,
		HarnessCompilerVersion: assemblycontract.CompilerVersionV1, GenerationRef: generation,
		ManifestRef: assemblycontract.ObjectRefV1{ID: publicationID + "/manifest", Revision: 1, Digest: digest("manifest")},
		GraphRef:    assemblycontract.ObjectRefV1{ID: publicationID + "/graph", Revision: 1, Digest: digest("graph")},
		HandoffRef:  assemblycontract.ObjectRefV1{ID: publicationID + "/handoff", Revision: 1, Digest: digest("handoff")},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestPackageIdentityIsOneToOneWithLock(t *testing.T) {
	lock := validLock(t)
	first, err := SealPackageV1(AgentPackageV1{CreatedUnixNano: 1, Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SealPackageV1(AgentPackageV1{CreatedUnixNano: 999, Lock: lock})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || first.CreatedUnixNano != lock.FrozenUnixNano {
		t.Fatalf("same lock produced different identity: %#v %#v", first, second)
	}
	forged := first
	forged.PackageID += "-other"
	forged.Digest, _ = PackageDigestV1(forged)
	if err := forged.Validate(); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("alternate ID for the same lock accepted: %v", err)
	}
}

func TestPackageIDUsesCompleteLockDigest(t *testing.T) {
	prefix := "sha256:0123456789abcdef01234567"
	left := core.Digest(prefix + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	right := core.Digest(prefix + "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if left.Validate() != nil || right.Validate() != nil {
		t.Fatal("test digests are invalid")
	}
	if packageIDV1(left) == packageIDV1(right) {
		t.Fatal("different lock digest suffixes collapsed to one package ID")
	}
	if packageIDV1(left) != "agent-package-"+strings.TrimPrefix(string(left), "sha256:") {
		t.Fatal("package ID does not preserve the complete normalized lock digest")
	}
}

func TestLockRejectsArtifactIDSwapEvenWithSameDigest(t *testing.T) {
	lock := validLock(t)
	lock.ManifestRef.ID = lock.GraphRef.ID
	lock.ManifestRef.Digest = lock.GraphRef.Digest
	lock.Digest = ""
	if _, err := SealLockManifestV1(lock); !core.HasReason(err, core.ReasonBindingDrift) {
		t.Fatalf("swapped artifact coordinate accepted: %v", err)
	}
}

func TestLockCanonicalizesReleaseOrderAndRejectsDuplicate(t *testing.T) {
	lock := validLock(t)
	second := lock.ComponentReleaseRefs[0]
	second.ReleaseID, second.ComponentID, second.Digest = "release/second", "component/second", digest("second")
	lock.ComponentReleaseRefs = []assemblercontract.ComponentReleaseRefV1{second, lock.ComponentReleaseRefs[0]}
	lock.Digest = ""
	a, err := SealLockManifestV1(lock)
	if err != nil {
		t.Fatal(err)
	}
	lock.ComponentReleaseRefs[0], lock.ComponentReleaseRefs[1] = lock.ComponentReleaseRefs[1], lock.ComponentReleaseRefs[0]
	b, err := SealLockManifestV1(lock)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("release set order changed lock")
	}
	lock.ComponentReleaseRefs = append(lock.ComponentReleaseRefs, lock.ComponentReleaseRefs[0])
	lock.Digest = ""
	if _, err := SealLockManifestV1(lock); !core.HasReason(err, core.ReasonDuplicateCanonicalKey) {
		t.Fatalf("duplicate release accepted: %v", err)
	}
}
