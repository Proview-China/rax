package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestLocalSyncTwoStageSettlementGateAndAtomicPublish(t *testing.T) {
	f := newFixture(t, false)
	request := PrepareLocalSyncRequest{
		PlanRef: ref("sync-plan"), PreparedID: "prepared-sync", AdmissionTTL: time.Hour, ExpiresAt: f.now.Add(time.Hour), ExpectedSource: contract.ExpectAbsent(), ExpectedPackage: contract.ExpectAbsent(),
		Source:  SourceInput{TenantID: f.access.TenantID, ID: "sync-source", Version: "v1", AssetRef: ref("sync-asset"), ContentDigest: f.contentRef.Digest, AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, License: "internal-use", Scope: "project-a", Sensitivity: "internal", State: SourceAvailable, Provenance: []contract.Ref{ref("sync-provenance")}, AcquiredAt: *f.now, ValidFrom: f.now.Add(-time.Hour), ValidTo: f.now.Add(24 * time.Hour)},
		Package: PackageInput{TenantID: f.access.TenantID, ID: "sync-package", Version: "v1", AuthorityRef: f.access.AuthorityRef, PolicyRef: f.access.PolicyRef, License: "internal-use", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: PackageReady},
		Records: []SyncRecordInput{{OperationRef: ref("sync-operation"), AttemptID: "sync-attempt", ExpectedRecord: contract.ExpectAbsent(), Candidate: CandidateInput{ID: "sync-candidate", ProducerID: "sync-controller", SourceEpoch: 1, SourceSequence: 1, Kind: CandidateRecord, PayloadDigest: "sha256:sync-payload", EvidenceRefs: []contract.Ref{ref("sync-evidence")}, TTL: time.Hour, Draft: RecordDraft{ID: "sync-record", ContentRef: f.contentRef, EvidenceRefs: []contract.Ref{ref("record-evidence")}, Scope: "project-a", Subject: "sync", Sensitivity: "internal", License: "internal-use", TrustState: TrustSourceSupported, ValidFrom: f.now.Add(-time.Hour), ValidTo: f.now.Add(12 * time.Hour)}}}},
	}
	prepared, err := f.store.PrepareLocalSync(f.access, request)
	if err != nil {
		t.Fatal(err)
	}
	finalRequest := FinalizeLocalSyncRequest{Prepared: prepared, Projections: []ProjectionInput{{ID: "sync-lexical", Kind: "lexical", BuilderVersion: "v1", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: ProjectionReady, TTL: time.Hour}}, Snapshot: SnapshotInput{ID: "sync-snapshot", Version: "v1", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}}, ExpectedSnapshot: contract.ExpectAbsent(), ExpectedPointer: contract.ExpectAbsent()}
	if _, err := f.store.FinalizeLocalSync(f.access, finalRequest); !errors.Is(err, contract.ErrNotFound) {
		t.Fatalf("unsettled finalized: %v", err)
	}
	f.store.mu.RLock()
	if f.store.tenants[f.access.TenantID].current != nil {
		t.Fatal("unsettled sync published current")
	}
	f.store.mu.RUnlock()
	for _, result := range prepared.DomainResults {
		association, err := contract.AssociateDomainResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.ApplySettlement(f.access, association, contract.RuntimeSettlementRef{Ref: ref("runtime-settlement-" + result.Ref.ID)}, contract.ExpectAbsent()); err != nil {
			t.Fatal(err)
		}
	}
	finalized, err := f.store.FinalizeLocalSync(f.access, finalRequest)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.PointerRef.Validate() != nil || finalized.PublishedSnapshotRef.Validate() != nil || len(finalized.ProjectionRefs) != 1 {
		t.Fatalf("final=%+v", finalized)
	}
}

func TestPreparedSyncTamperFailsBeforeProjection(t *testing.T) {
	f := newFixture(t, false)
	prepared := PreparedSyncV1{ContractVersion: SyncContractVersionV1, ObjectKind: PreparedSyncObjectKindV1, Ref: ref("prepared"), Owner: contract.OwnerKnowledge, TenantID: f.access.TenantID, PlanRef: ref("plan"), SourceRef: f.source.Ref, PackageRef: f.pkg.Ref, DomainResults: []contract.DomainResultFact{f.result}, RecordRefs: []contract.Ref{ref("wrong")}, CreatedAt: *f.now, ExpiresAt: f.now.Add(time.Hour), Digest: "sha256:wrong"}
	if _, err := f.store.FinalizeLocalSync(f.access, FinalizeLocalSyncRequest{Prepared: prepared}); err == nil {
		t.Fatal("tampered prepared sync accepted")
	}
}
