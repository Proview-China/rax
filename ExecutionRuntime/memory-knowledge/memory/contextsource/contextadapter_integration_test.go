package contextsource_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory/contextadapter"
	"github.com/Proview-China/rax/ExecutionRuntime/memory-knowledge/memory/contextsource"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
)

func TestMemoryApplicationAdapterUsesOnlyV2OwnerReader(t *testing.T) {
	fixture := contextsource.NewAdapterTestFixtureV2(t)
	adapter, err := contextadapter.NewAdapterV1(fixture.Reader, fixture.Now)
	if err != nil {
		t.Fatal(err)
	}
	request := memoryApplicationRequestV1(t, fixture.Request)
	s1, err := adapter.InspectContextOwnerSourceCurrentV1(context.Background(), request)
	if err != nil || s1.Owner != applicationcontract.ContextOwnerMemoryV1 || len(s1.Items) != 1 {
		t.Fatalf("s1=%+v err=%v", s1, err)
	}
	otherTurn := core.DigestBytes([]byte("another-turn"))
	replay := request
	replay.SourceTurn.ID, replay.SourceTurn.Digest = "turn:"+string(otherTurn), otherTurn
	replay.TurnApplicability.ID, replay.TurnApplicability.Digest = replay.SourceTurn.ID, otherTurn
	replay.Digest = ""
	replay, err = applicationcontract.SealContextOwnerSourceRequestV1(replay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.InspectContextOwnerSourceCurrentV1(context.Background(), replay); err == nil {
		t.Fatal("cross-Turn exact replay was accepted")
	}
	otherSessionEvidence := core.DigestBytes([]byte("another-session-applicability"))
	evidenceReplay := request
	evidenceReplay.SessionApplicability.ID, evidenceReplay.SessionApplicability.Digest = "session:"+string(otherSessionEvidence), otherSessionEvidence
	evidenceReplay.Digest = ""
	evidenceReplay, err = applicationcontract.SealContextOwnerSourceRequestV1(evidenceReplay)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adapter.InspectContextOwnerSourceCurrentV1(context.Background(), evidenceReplay); err == nil {
		t.Fatal("Session applicability evidence substitution was accepted")
	}
	fixture.SetNow(fixture.Now().Add(time.Second))
	s2Request := request
	s2Request.Phase = applicationcontract.ContextSourceCheckS2V1
	s2Request.ExpectedOwnerClosure = s1.StableClosureDigest
	s2Request.ExpectedStableDigest = s1.StableAssociationDigest
	s2Request.Digest = ""
	s2Request, err = applicationcontract.SealContextOwnerSourceRequestV1(s2Request)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := adapter.InspectContextOwnerSourceCurrentV1(context.Background(), s2Request)
	if err != nil || s2.StableAssociationDigest != s1.StableAssociationDigest || s2.Digest == s1.Digest {
		t.Fatalf("s2=%+v err=%v", s2, err)
	}
	contentRequest, err := applicationcontract.SealContextOwnerContentRequestV1(applicationcontract.ContextOwnerContentRequestV1{SourceRequest: s2Request, Envelope: s2, Rank: 0, MaxBodyBytes: s2.Items[0].ContentRef.Length, RequestedNotAfterNano: fixture.Now().Add(5 * time.Minute).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	observation, body, err := adapter.ReadContextOwnerContentExactV1(context.Background(), contentRequest)
	if err != nil || string(body) != "owner local memory content" || observation.ProjectionItemDigest != s2.Items[0].ItemDigest {
		t.Fatalf("observation=%+v body=%q err=%v", observation, body, err)
	}
	body[0] = 'X'
	_, again, err := adapter.ReadContextOwnerContentExactV1(context.Background(), contentRequest)
	if err != nil || string(again) != "owner local memory content" {
		t.Fatalf("copy isolation failed: %q %v", again, err)
	}

	tampered := contentRequest
	tampered.Envelope.Items = append([]applicationcontract.ContextOwnerSourceItemV1(nil), contentRequest.Envelope.Items...)
	tampered.Envelope.Items[0].RecordRef.Digest = core.DigestBytes([]byte("tamper"))
	if _, body, err := adapter.ReadContextOwnerContentExactV1(context.Background(), tampered); err == nil || body != nil {
		t.Fatal("tampered nested record ref was accepted")
	}
}

func memoryApplicationRequestV1(t *testing.T, owner contextsource.CurrentRequestV2) applicationcontract.ContextOwnerSourceRequestV1 {
	t.Helper()
	payload, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	coordinate := owner.Coordinate
	sessionDigest := core.Digest(coordinate.SessionRef.Digest)
	turnDigest := core.Digest(coordinate.SourceTurnRef.Digest)
	request, err := applicationcontract.SealContextOwnerSourceRequestV1(applicationcontract.ContextOwnerSourceRequestV1{
		Owner:                applicationcontract.ContextOwnerMemoryV1,
		SourceSession:        applicationcontract.SingleCallSessionCoordinateV1{ID: coordinate.SessionRef.ID, Revision: core.Revision(coordinate.SessionRef.Revision), Digest: sessionDigest, Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: coordinate.SessionCheckedAt.UnixNano(), ExpiresUnixNano: coordinate.SessionExpiresAt.UnixNano()},
		SessionApplicability: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: coordinate.SessionEvidenceRef.ID, Revision: core.Revision(coordinate.SessionEvidenceRef.Revision), Digest: core.Digest(coordinate.SessionEvidenceRef.Digest)},
		SourceTurn:           applicationcontract.SingleCallTurnCoordinateV1{ID: coordinate.SourceTurnRef.ID, Ordinal: coordinate.SourceTurnOrdinal, Revision: core.Revision(coordinate.SourceTurnRef.Revision), Digest: turnDigest},
		TurnApplicability:    applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: coordinate.TurnEvidenceRef.ID, Revision: core.Revision(coordinate.TurnEvidenceRef.Revision), Digest: core.Digest(coordinate.TurnEvidenceRef.Digest)},
		OwnerRequest:         payload, Phase: applicationcontract.ContextSourceCheckS1V1, RequestedNotAfterNano: owner.NotAfter.UnixNano(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return request
}
