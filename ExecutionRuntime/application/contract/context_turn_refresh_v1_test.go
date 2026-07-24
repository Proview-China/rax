package contract

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

func contextRefreshDigestV1(seed string) core.Digest { return core.DigestBytes([]byte(seed)) }

func contextRefreshRefV1(kind, id string) ContextRefreshExactRefV1 {
	return ContextRefreshExactRefV1{Kind: runtimeports.NamespacedNameV2(kind), ID: id, Revision: 1, Digest: contextRefreshDigestV1(id)}
}

func contextRefreshTurnV1() (SingleCallTurnCoordinateV1, SingleCallTurnApplicabilitySourceCoordinateV1) {
	digest := contextRefreshDigestV1("turn")
	id := "turn:" + string(digest)
	return SingleCallTurnCoordinateV1{ID: id, Ordinal: 7, Revision: 1, Digest: digest}, SingleCallTurnApplicabilitySourceCoordinateV1{Kind: SingleCallTurnSourceKindV1, ID: id, Revision: 1, Digest: digest}
}

func contextRefreshSessionV1(now time.Time) (SingleCallSessionCoordinateV1, SingleCallSessionApplicabilitySourceCoordinateV1) {
	digest := contextRefreshDigestV1("session")
	return SingleCallSessionCoordinateV1{ID: "session-1", Revision: 1, Digest: digest, Phase: SingleCallSessionWaitingActionV1, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano()}, SingleCallSessionApplicabilitySourceCoordinateV1{Kind: SingleCallSessionSourceKindV1, ID: "session:" + string(digest), Revision: 1, Digest: digest}
}

func contextRefreshEnvelopeV1(t *testing.T, now time.Time, phase ContextSourceCheckPhaseV1) ContextOwnerSourceEnvelopeV1 {
	t.Helper()
	turn, applicability := contextRefreshTurnV1()
	session, sessionApplicability := contextRefreshSessionV1(now)
	item, err := SealContextOwnerSourceItemV1(ContextOwnerSourceItemV1{
		Rank: 0, ItemDigest: contextRefreshDigestV1("item-1"), RecordRef: contextRefreshRefV1("memory/record", "record-1"),
		StableOwnerChain: []ContextRefreshExactRefV1{contextRefreshRefV1("memory/source", "source-1")}, ContentRef: ContextOwnerContentRefV1{ID: "content-1", Digest: contextRefreshDigestV1("content-1"), Length: 12, MediaType: "text/plain"},
		TokenEstimate: 3, Sensitivity: "internal", CitationDigest: contextRefreshDigestV1("citation"), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := SealContextOwnerSourceEnvelopeV1(ContextOwnerSourceEnvelopeV1{
		ID: "memory-envelope-1", Owner: ContextOwnerMemoryV1, SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: applicability,
		AttemptInspectionRef: contextRefreshRefV1("memory/inspection", "inspection-1"), CurrentProjectionRef: contextRefreshRefV1("memory/projection", "projection-1"),
		StableClosureDigest: contextRefreshDigestV1("closure"), Items: []ContextOwnerSourceItemV1{item}, Phase: phase,
		CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(time.Minute).UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestContextOwnerSourceStableAndFreshDigests(t *testing.T) {
	now := time.Unix(1700000000, 0)
	s1 := contextRefreshEnvelopeV1(t, now, ContextSourceCheckS1V1)
	if err := s1.ValidateCurrent(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	s2 := s1
	s2.Phase = ContextSourceCheckS2V1
	s2.CheckedUnixNano = now.Add(2 * time.Second).UnixNano()
	s2.AttemptInspectionRef = contextRefreshRefV1("memory/inspection", "inspection-2")
	s2.CurrentProjectionRef = contextRefreshRefV1("memory/projection", "projection-2")
	var err error
	s2, err = SealContextOwnerSourceEnvelopeV1(s2)
	if err != nil {
		t.Fatal(err)
	}
	if s1.StableAssociationDigest != s2.StableAssociationDigest {
		t.Fatal("fresh refs changed the stable association")
	}
	if s1.Digest == s2.Digest {
		t.Fatal("fresh projection did not change the envelope digest")
	}

	tampered := s2
	tampered.Items = append([]ContextOwnerSourceItemV1(nil), s2.Items...)
	tampered.Items[0].ContentRef.Length++
	if err := tampered.ValidateCurrent(now.Add(3 * time.Second)); err == nil {
		t.Fatal("tampered nested item was accepted")
	}
}

func TestContextOwnerSourceRequestAndBoundedContent(t *testing.T) {
	now := time.Unix(1700000000, 0)
	turn, applicability := contextRefreshTurnV1()
	session, sessionApplicability := contextRefreshSessionV1(now)
	req, err := SealContextOwnerSourceRequestV1(ContextOwnerSourceRequestV1{Owner: ContextOwnerMemoryV1, SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: applicability, OwnerRequest: []byte(`{"owner":"memory"}`), Phase: ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := req.ValidateCurrent(now); err != nil {
		t.Fatal(err)
	}
	req.OwnerRequest[0] ^= 1
	if err := req.ValidateCurrent(now); err == nil {
		t.Fatal("tampered opaque owner request was accepted")
	}

	envelope := contextRefreshEnvelopeV1(t, now, ContextSourceCheckS1V1)
	sourceRequest, err := SealContextOwnerSourceRequestV1(ContextOwnerSourceRequestV1{Owner: ContextOwnerMemoryV1, SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: applicability, OwnerRequest: []byte(`{"owner":"memory"}`), Phase: ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	content, err := SealContextOwnerContentRequestV1(ContextOwnerContentRequestV1{SourceRequest: sourceRequest, Envelope: envelope, Rank: 0, MaxBodyBytes: 12, RequestedNotAfterNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := content.ValidateCurrent(now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	content.MaxBodyBytes = 13
	if err := content.ValidateCurrent(now.Add(time.Second)); err == nil {
		t.Fatal("content request exceeded the exact item bound")
	}
}

func TestContextOwnerSourceRejectsExactTurnSubstitution(t *testing.T) {
	now := time.Unix(1700000000, 0)
	turn, _ := contextRefreshTurnV1()
	session, sessionApplicability := contextRefreshSessionV1(now)

	otherTurnDigest := contextRefreshDigestV1("other-turn")
	otherTurn := SingleCallTurnApplicabilitySourceCoordinateV1{Kind: SingleCallTurnSourceKindV1, ID: "turn:" + string(otherTurnDigest), Revision: 1, Digest: otherTurnDigest}
	request, err := SealContextOwnerSourceRequestV1(ContextOwnerSourceRequestV1{Owner: ContextOwnerMemoryV1, SourceSession: session, SessionApplicability: sessionApplicability, SourceTurn: turn, TurnApplicability: otherTurn, OwnerRequest: []byte(`{"owner":"memory"}`), Phase: ContextSourceCheckS1V1, RequestedNotAfterNano: now.Add(time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if err := request.ValidateCurrent(now); err == nil {
		t.Fatal("a valid but different Turn applicability was accepted")
	}

	envelope := contextRefreshEnvelopeV1(t, now, ContextSourceCheckS1V1)
	envelope.TurnApplicability = otherTurn
	envelope, err = SealContextOwnerSourceEnvelopeV1(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if err := envelope.ValidateCurrent(now); err == nil {
		t.Fatal("an envelope with substituted Turn applicability was accepted")
	}
}
