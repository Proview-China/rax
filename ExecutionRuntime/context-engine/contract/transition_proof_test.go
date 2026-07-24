package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testfixture"
	"github.com/Proview-China/rax/ExecutionRuntime/context-engine/internal/testkit"
)

func TestTransitionProofStableFreshSeparationAndTamper(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	now := fixture.Now.UnixNano()
	applicationAttempt := contract.FactRef{ID: "application-attempt-1", Revision: 1, Digest: testkit.D("application-attempt")}
	refreshAttempt := contract.FactRef{ID: fixture.Request.RefreshAttemptID, Revision: 1, Digest: fixture.Request.Digest}
	typedSession := contract.ContextTypedFactRefV1{Kind: "harness/session", Ref: fixture.Request.ExpectedCurrent.SessionRef}
	typedTurn := contract.ContextTypedFactRefV1{Kind: "harness/turn", Ref: contract.FactRef{ID: "turn-1", Revision: 1, Digest: testkit.D("turn")}}
	transition, err := contract.SealContextTurnTransitionRequestV1(contract.ContextTurnTransitionRequestV1{ApplicationAttemptRef: applicationAttempt, RefreshAttemptRef: refreshAttempt, SourceSessionRef: typedSession, SourceTurnRef: typedTurn, SourceTurnOrdinal: fixture.Request.ExpectedCurrent.Turn, ExpectedTargetOrdinal: fixture.Request.ExpectedCurrent.Turn + 1, ExpectedCurrent: fixture.Request.ExpectedCurrent, StableSourceSetDigest: testkit.D("stable-source-set"), S1AssociationSetDigest: testkit.D("s1-association"), CheckedUnixNano: now, ExpiresUnixNano: now + int64(10*time.Second)}, now)
	if err != nil {
		t.Fatal(err)
	}
	transitionRef, _ := transition.Ref()
	child := fixture.Request.ToolSource.Execution
	child.Turn++
	proof, err := contract.SealContextTurnTransitionProofV1(contract.ContextTurnTransitionProofV1{TransitionRequestRef: transitionRef, ApplicationAttemptRef: applicationAttempt, RefreshAttemptRef: refreshAttempt, SourceSessionRef: typedSession, SourceTurnRef: typedTurn, SourceTurnOrdinal: transition.SourceTurnOrdinal, TargetTurnOrdinal: transition.ExpectedTargetOrdinal, ExpectedCurrent: transition.ExpectedCurrent, ChildExecution: child, PendingDomainResultRef: factV1("pending"), ManifestRef: factV1("manifest"), FrameRef: factV1("frame"), GenerationRef: factV1("generation"), StableSourceSetDigest: transition.StableSourceSetDigest})
	if err != nil {
		t.Fatal(err)
	}
	first, err := contract.SealContextTurnTransitionProofCurrentV1(contract.ContextTurnTransitionProofCurrentV1{Proof: proof, S1AssociationSetDigest: transition.S1AssociationSetDigest, CheckedUnixNano: now, ExpiresUnixNano: now + int64(5*time.Second)}, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := contract.SealContextTurnTransitionProofCurrentV1(contract.ContextTurnTransitionProofCurrentV1{Proof: proof, S1AssociationSetDigest: transition.S1AssociationSetDigest, CheckedUnixNano: now + int64(time.Second), ExpiresUnixNano: now + int64(6*time.Second)}, now+int64(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if first.Proof.Digest != second.Proof.Digest || first.Digest == second.Digest {
		t.Fatal("fresh TTL changed stable proof or failed to change current projection")
	}
	tampered := proof
	tampered.FrameRef = factV1("another-frame")
	if err := tampered.Validate(); err == nil {
		t.Fatal("tampered exact Frame ref was accepted")
	}
	if err := first.ValidateAt(first.ExpiresUnixNano); err == nil {
		t.Fatal("expired proof current projection was accepted")
	}
}

func factV1(id string) contract.FactRef {
	return contract.FactRef{ID: id, Revision: 1, Digest: testkit.D(id)}
}

func TestApplyTransitionBindingIsAllOrNoneAndCanonical(t *testing.T) {
	fixture, err := testfixture.NewRefreshFixtureV1()
	if err != nil {
		t.Fatal(err)
	}
	now := fixture.Now.UnixNano()
	base := contract.ApplyContextTurnRefreshRequestV1{
		AttemptRef:             factV1("refresh-attempt"),
		PendingDomainResultRef: factV1("pending"),
		ExpectedCurrent:        fixture.Request.ExpectedCurrent,
		CheckedUnixNano:        now,
		NotAfterUnixNano:       now + int64(time.Second),
	}
	legacy, err := contract.SealApplyContextTurnRefreshRequestV1(base, now)
	if err != nil || legacy.ValidateAt(now) != nil {
		t.Fatalf("legacy all-absent binding rejected: %v", err)
	}
	proof := factV1("transition-proof")
	bound := base
	bound.TransitionProofRef = &proof
	bound.StableSourceSetDigest = testkit.D("stable-source-set")
	bound.S2AssociationSetDigest = testkit.D("s2-association-set")
	sealed, err := contract.SealApplyContextTurnRefreshRequestV1(bound, now)
	if err != nil || sealed.ValidateAt(now) != nil {
		t.Fatalf("complete transition binding rejected: %v", err)
	}
	partial := base
	partial.TransitionProofRef = &proof
	partial, err = contract.SealApplyContextTurnRefreshRequestV1(partial, now)
	if err == nil || partial.ValidateAt(now) == nil {
		t.Fatal("partial transition binding was accepted")
	}
	tampered := sealed
	tampered.S2AssociationSetDigest = testkit.D("tampered-s2")
	if tampered.ValidateAt(now) == nil {
		t.Fatal("canonical S2 association tamper was accepted")
	}
}
