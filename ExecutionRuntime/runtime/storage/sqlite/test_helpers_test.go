package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func openTestStore(t *testing.T, path string, clock func() time.Time) *Store {
	t.Helper()
	store, err := Open(context.Background(), Config{Path: path, BusyTimeout: time.Second, MaxOpenConns: 8, Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testDBPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "runtime-binding.db")
}

func (s *Store) failNextStageForTest() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.failNextStage = true
}

func (s *Store) loseNextReplyForTest() {
	s.faultMu.Lock()
	defer s.faultMu.Unlock()
	s.loseNextReply = true
}

func testDigest(t *testing.T, value any) core.Digest {
	t.Helper()
	digest, err := core.DigestJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func certifiedBinding(t *testing.T, store *Store, base time.Time, setID, bindingID string, component ports.ComponentIDV2, capability ports.CapabilityNameV2) (control.BindingFactV2, control.BindingSetFactV2) {
	t.Helper()
	artifact := testDigest(t, "artifact-"+string(component))
	manifest := ports.ComponentManifestV2{
		ContractVersion: ports.BindingContractVersionV2, ComponentID: component, Kind: "runtime/component", GovernanceCategory: "runtime/review", SemanticVersion: "1.0.0", ArtifactDigest: artifact,
		Contract: ports.ContractBindingV2{Name: "runtime/review-binding", Version: "1.0.0", Compatible: ports.VersionRangeV2{MinimumInclusive: "1.0.0", MaximumExclusive: "2.0.0"}},
		Schemas:  []ports.SchemaRefV2{}, Locality: ports.LocalityHostControlPlane, Dependencies: []ports.ComponentDependencyV2{}, RequiredCapabilities: []ports.CapabilityRequirementV2{},
		ProvidedCapabilities: []ports.ProvidedCapabilityV2{{Capability: capability, TTLSeconds: 300, Schemas: []ports.SchemaRefV2{}}}, Conformance: ports.ConformanceFullyControlled, ResidualClass: ports.ResidualInspectable,
		Owners:      []ports.OwnerAssignmentV2{{Role: ports.OwnerEffect, OwnerComponentID: component}, {Role: ports.OwnerSettlement, OwnerComponentID: component}, {Role: ports.OwnerCleanup, OwnerComponentID: component}},
		Credentials: []ports.CredentialRequirementV2{}, OfflinePolicy: ports.OfflineDenied, Extensions: []ports.GovernanceExtensionV2{}, Annotations: []ports.DisplayAnnotationV2{},
	}
	manifestDigest, err := manifest.BindingDigestV2()
	if err != nil {
		t.Fatal(err)
	}
	governance := testDigest(t, "governance-"+setID)
	expires := base.Add(5 * time.Minute).UnixNano()
	grant := ports.CapabilityGrantV2{Capability: capability, EvidenceDigest: testDigest(t, "grant-"+bindingID), ObservedUnixNano: base.UnixNano(), ExpiresUnixNano: expires}
	certified := control.BindingFactV2{ID: bindingID, ComponentID: component, Manifest: manifest, ManifestDigest: manifestDigest, GovernanceDigest: governance, State: control.BindingCertified, Revision: 3, Grants: []ports.CapabilityGrantV2{grant}, ProbedUnixNano: base.UnixNano(), CertifiedUnixNano: base.Add(time.Second).UnixNano(), ConformanceEvidenceDigest: testDigest(t, "conformance-"+bindingID), ExpiresUnixNano: expires, RenewalEvidence: []ports.EvidenceRecordRefV2{}}
	declared := certified
	declared.State, declared.Revision, declared.Grants = control.BindingDeclared, 1, []ports.CapabilityGrantV2{}
	declared.ProbedUnixNano, declared.CertifiedUnixNano, declared.ExpiresUnixNano, declared.ConformanceEvidenceDigest = 0, 0, 0, ""
	if _, err := store.CreateBinding(context.Background(), declared); err != nil {
		t.Fatal(err)
	}
	probed := certified
	probed.State, probed.Revision, probed.CertifiedUnixNano, probed.ConformanceEvidenceDigest = control.BindingProbed, 2, 0, ""
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 1, Next: probed}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareAndSwapBinding(context.Background(), control.BindingFactCASRequestV2{ExpectedRevision: 2, Next: certified}); err != nil {
		t.Fatal(err)
	}
	set := control.BindingSetFactV2{ID: setID, PlanID: "plan-" + setID, PlanDigest: testDigest(t, "plan-"+setID), GovernanceDigest: governance, State: control.BindingSetActive, Revision: 1, Members: []control.BindingMemberV2{{BindingID: bindingID, BindingRevision: certified.Revision, ComponentID: component, Kind: manifest.Kind, ManifestDigest: manifestDigest, ArtifactDigest: artifact, Contract: manifest.Contract, Owners: append([]ports.OwnerAssignmentV2(nil), manifest.Owners...), Grants: []ports.CapabilityGrantV2{grant}}}, TopologicalOrder: []ports.ComponentIDV2{component}, Residuals: []control.BindingResidualV2{}, CreatedUnixNano: base.Add(time.Second).UnixNano(), ExpiresUnixNano: expires}
	return certified, set
}

func commitComponent(t *testing.T, store *Store, base time.Time, setID, bindingID string, component ports.ComponentIDV2, capability ports.CapabilityNameV2) (control.BindingSetFactV2, control.BindingFactV2) {
	t.Helper()
	certified, set := certifiedBinding(t, store, base, setID, bindingID, component, capability)
	committed, err := store.CommitBindingSet(context.Background(), control.CommitBindingSetRequestV2{Set: set, Expected: []control.ExpectedBindingRevisionV2{{BindingID: bindingID, ExpectedRevision: certified.Revision}}})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := store.InspectBinding(context.Background(), bindingID)
	if err != nil {
		t.Fatal(err)
	}
	return committed, bound
}
