package review_test

import (
	"context"
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/review/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/review/internal/testkit"
	"github.com/Proview-China/rax/ExecutionRuntime/review/memory"
	reviewport "github.com/Proview-China/rax/ExecutionRuntime/review/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/review/service"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func TestBehaviorFeedbackCandidatePersistsExactResolvedProvenanceV1(t *testing.T) {
	flow := newResolvedFlow(t, 10*time.Minute)
	value := feedbackCandidateV1(t, flow)
	created, err := flow.store.CreateBehaviorFeedbackCandidateV1(context.Background(), value)
	if err != nil || created.Digest != value.Digest {
		t.Fatalf("create behavior feedback failed: %+v %v", created, err)
	}
	inspected, err := flow.store.InspectBehaviorFeedbackCandidateExactV1(context.Background(), value.TenantID, reviewport.ExactV1(value.ID, value.Revision, value.Digest))
	if err != nil || inspected.Digest != value.Digest {
		t.Fatalf("inspect behavior feedback failed: %+v %v", inspected, err)
	}
	drift := value
	drift.ID = "feedback-reviewer-drift"
	drift.ReviewerID = "another-reviewer"
	drift.Digest = ""
	drift, _ = contract.SealBehaviorFeedbackCandidateV1(drift)
	if _, err := flow.store.CreateBehaviorFeedbackCandidateV1(context.Background(), drift); !core.HasCategory(err, core.ErrorConflict) {
		t.Fatalf("reviewer provenance drift was accepted: %v", err)
	}
	if _, err := flow.store.InspectBehaviorFeedbackCandidateExactV1(context.Background(), drift.TenantID, reviewport.ExactV1(drift.ID, drift.Revision, drift.Digest)); !core.HasCategory(err, core.ErrorNotFound) {
		t.Fatalf("failed feedback create leaked candidate: %v", err)
	}
}

func feedbackCandidateV1(t *testing.T, flow *resolvedFlow) contract.BehaviorFeedbackCandidateV1 {
	t.Helper()
	evidence := []runtimeports.ReviewEvidenceRefV2{flow.attestation.Evidence[0]}
	evidenceDigest, err := contract.ComputeReviewEvidenceDigestV1(evidence)
	if err != nil {
		t.Fatal(err)
	}
	value, err := contract.SealBehaviorFeedbackCandidateV1(contract.BehaviorFeedbackCandidateV1{
		FactIdentityV1: contract.FactIdentityV1{ContractVersion: contract.ContractVersionV1, TenantID: flow.resolved.TenantID, ID: "feedback-resolved", Revision: 1, CreatedUnixNano: flow.clock.Now().UnixNano(), UpdatedUnixNano: flow.clock.Now().UnixNano()},
		Case:           contract.ExactResourceRefV1{ID: flow.resolved.ID, Revision: flow.resolved.Revision, Digest: flow.resolved.Digest}, Target: contract.ExactResourceRefV1{ID: flow.target.ID, Revision: flow.target.Revision, Digest: flow.target.Digest}, Verdict: contract.ExactResourceRefV1{ID: flow.verdict.ID, Revision: flow.verdict.Revision, Digest: flow.verdict.Digest},
		Policy: flow.verdict.Policy, ReviewerID: flow.verdict.ReviewerID, ReviewerBinding: flow.verdict.ReviewerBinding, BehaviorClass: "praxis.review/repeated-unsafe-action", SignalDigest: testkit.Digest("feedback-signal"), Evidence: evidence, EvidenceDigest: evidenceDigest, ExpiresUnixNano: flow.clock.Now().Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

type feedbackReplyLostStoreV1 struct {
	*memory.Store
	calls int
}

func (s *feedbackReplyLostStoreV1) CreateBehaviorFeedbackCandidateV1(ctx context.Context, value contract.BehaviorFeedbackCandidateV1) (contract.BehaviorFeedbackCandidateV1, error) {
	s.calls++
	created, err := s.Store.CreateBehaviorFeedbackCandidateV1(ctx, value)
	if err != nil {
		return created, err
	}
	return contract.BehaviorFeedbackCandidateV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEffectUnknownOutcome, "injected behavior feedback reply loss")
}

func TestBehaviorFeedbackLostReplyInspectsOriginalCandidateV1(t *testing.T) {
	flow := newResolvedFlow(t, 10*time.Minute)
	store := &feedbackReplyLostStoreV1{Store: flow.store}
	owner, err := service.New(store, flow.clock.Now)
	if err != nil {
		t.Fatal(err)
	}
	value := feedbackCandidateV1(t, flow)
	created, err := owner.CreateBehaviorFeedbackCandidateV1(context.Background(), value)
	if err != nil || created.Digest != value.Digest || store.calls != 1 {
		t.Fatalf("behavior feedback lost-reply recovery calls=%d value=%+v err=%v", store.calls, created, err)
	}
}
