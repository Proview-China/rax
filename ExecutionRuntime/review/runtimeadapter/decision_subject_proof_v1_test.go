package runtimeadapter

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

type decisionProofStoreV1 struct {
	reviewport.StoreV1
	target     contract.TargetSnapshotV1
	assignment contract.ReviewerAssignmentV1
}

func (s *decisionProofStoreV1) InspectTargetExactV1(_ context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.TargetSnapshotV1, error) {
	if tenant != s.target.TenantID || ref != reviewport.ExactV1(s.target.ID, s.target.Revision, s.target.Digest) {
		return contract.TargetSnapshotV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "target not found")
	}
	return s.target, nil
}

func (s *decisionProofStoreV1) InspectAssignmentExactV1(_ context.Context, tenant core.TenantID, ref reviewport.ExactFactRefV1) (contract.ReviewerAssignmentV1, error) {
	if tenant != s.assignment.TenantID || ref != reviewport.ExactV1(s.assignment.ID, s.assignment.Revision, s.assignment.Digest) {
		return contract.ReviewerAssignmentV1{}, core.NewError(core.ErrorNotFound, core.ReasonInvalidReference, "assignment not found")
	}
	return s.assignment, nil
}

func TestDecisionSubjectProofReaderV1UsesTenantQualifiedExactInspect(t *testing.T) {
	now := time.Unix(1_820_000_000, 0)
	target := testkit.Target(now)
	caseFact := contract.ReviewCaseV1{FactIdentityV1: contract.FactIdentityV1{TenantID: target.TenantID, ID: "case-proof", Revision: 2}, TargetID: target.ID, TargetRevision: target.Revision, TargetDigest: target.Digest}
	round := testkit.Round(now, caseFact, contract.RouteHumanV1)
	assignment := testkit.Assignment(now, caseFact, round, contract.RouteHumanV1)
	reader, err := NewDecisionSubjectProofReaderV1(&decisionProofStoreV1{target: target, assignment: assignment})
	if err != nil {
		t.Fatal(err)
	}
	targetRef := runtimeports.ReviewDecisionTargetRefV1{TenantID: target.TenantID, ID: target.ID, Revision: target.Revision, Digest: target.Digest, RunID: target.RunID}
	if got, err := reader.InspectReviewDecisionTargetProofV1(context.Background(), targetRef); err != nil || got != targetRef {
		t.Fatalf("exact Target proof failed: got=%+v err=%v", got, err)
	}
	assignmentRef := runtimeports.ReviewDecisionAssignmentRefV1{TenantID: assignment.TenantID, ID: assignment.ID, Revision: assignment.Revision, Digest: assignment.Digest, ReviewerID: assignment.ReviewerID}
	if got, err := reader.InspectReviewDecisionAssignmentProofV1(context.Background(), assignmentRef); err != nil || got != assignmentRef {
		t.Fatalf("exact Assignment proof failed: got=%+v err=%v", got, err)
	}
	assignmentRef.TenantID = "tenant-other"
	if _, err := reader.InspectReviewDecisionAssignmentProofV1(context.Background(), assignmentRef); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("cross-tenant Assignment lookup did not fail closed: %v", err)
	}
}

func TestDecisionSubjectProofReaderV1RejectsTypedNilStore(t *testing.T) {
	var store *decisionProofStoreV1
	if _, err := NewDecisionSubjectProofReaderV1(store); !core.HasReason(err, core.ReasonComponentMissing) {
		t.Fatalf("typed-nil Store was accepted: %v", err)
	}
}
