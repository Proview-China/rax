package knowledge

import (
	"errors"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/contract"
)

func TestKnowledgePrepareSourcePurgeListsDerivedBytesNotAsset(t *testing.T) {
	f := newFixture(t, true)
	projection, err := f.store.PutProjection(f.access, ProjectionInput{TenantID: f.access.TenantID, ID: "projection-purge", Kind: "lexical", RecordRefs: []contract.Ref{f.record.Ref}, BuilderVersion: "v1", Coverage: contract.Coverage{Status: contract.CoverageComplete, Expected: 1, Available: 1}, State: ProjectionReady, TTL: time.Hour}, contract.ExpectAbsent())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.WithdrawSource(f.access, f.source.Ref.ID, "revoked", contract.ExpectRevision(f.source.Ref.Revision)); err != nil {
		t.Fatal(err)
	}
	request := PurgeRequest{ID: "source-purge", TargetKind: "source", TargetRef: f.source.Ref, ScopeRef: ref("scope"), OperationRef: ref("purge-operation"), RequestedByRef: ref("requester"), RetentionDecisionRef: ref("retention-clear"), LegalHoldInspectionRef: ref("legal-hold-clear"), TTL: time.Hour}
	intent, err := f.store.PreparePurge(f.access, request)
	if err != nil || len(intent.ContentRefs) != 1 || intent.ContentRefs[0] != f.record.ContentRef || len(intent.ProjectionRefs) != 1 || !contract.SameRef(intent.ProjectionRefs[0], projection.Ref) || contract.SameRef(intent.TargetRef, f.assetRef) {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
	inspected, err := f.store.InspectPurge(f.access, intent.Ref)
	if err != nil || !contract.SameRef(inspected.Ref, intent.Ref) {
		t.Fatalf("inspect=%+v err=%v", inspected, err)
	}
	retry, err := f.store.PreparePurge(f.access, request)
	if err != nil || !contract.SameRef(retry.Ref, intent.Ref) {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
}

func TestKnowledgePreparePurgeFailClosedBeforeWithdrawalAndOnDrift(t *testing.T) {
	f := newFixture(t, true)
	request := PurgeRequest{ID: "source-purge", TargetKind: "source", TargetRef: f.source.Ref, ScopeRef: ref("scope"), OperationRef: ref("purge-operation"), RequestedByRef: ref("requester"), RetentionDecisionRef: ref("retention-clear"), LegalHoldInspectionRef: ref("legal-hold-clear"), TTL: time.Hour}
	if _, err := f.store.PreparePurge(f.access, request); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("available source purge prepared: %v", err)
	}
	if _, _, err := f.store.WithdrawSource(f.access, f.source.Ref.ID, "revoked", contract.ExpectRevision(f.source.Ref.Revision)); err != nil {
		t.Fatal(err)
	}
	drift := request
	drift.TargetRef.Digest = "sha256:other"
	if _, err := f.store.PreparePurge(f.access, drift); !errors.Is(err, contract.ErrNotCurrent) {
		t.Fatalf("drift target accepted: %v", err)
	}
}
