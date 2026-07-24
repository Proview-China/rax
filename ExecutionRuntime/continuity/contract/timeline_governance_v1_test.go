package contract_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
)

func TestTimelineProjectionRequestV1IsCoordinateOnlyAndCanonical(t *testing.T) {
	request := timelineProjectionRequestV1(t)
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	typeOf := reflect.TypeOf(request)
	forbidden := []string{"semantic", "payload", "trust", "sequence", "current", "admitted", "inspected", "observed", "recorded"}
	for index := 0; index < typeOf.NumField(); index++ {
		name := strings.ToLower(typeOf.Field(index).Name)
		for _, fragment := range forbidden {
			if strings.Contains(name, fragment) && name != "requestednotafter" {
				t.Fatalf("caller request contains trusted field %s", typeOf.Field(index).Name)
			}
		}
	}
	tampered := request.Clone()
	tampered.EvidenceSource.SourceSequence++
	if err := tampered.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("same request digest accepted source drift: %v", err)
	}
	tampered = request.Clone()
	tampered.RequestedNotAfter = -1
	tampered.Digest = ""
	tampered.Digest, _ = tampered.CanonicalDigest()
	if err := tampered.Validate(); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("negative caller bound accepted: %v", err)
	}
}

func TestTimelineProjectionAttemptV1RequiresExactCASProgression(t *testing.T) {
	request := timelineProjectionRequestV1(t)
	proposed, err := contract.SealTimelineProjectionAttemptV1(contract.TimelineProjectionAttemptFactV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Ref:             contract.TimelineProjectionAttemptRefV1{AttemptID: request.AttemptID, Revision: 1, ScopeDigest: request.ScopeDigest},
		Request:         request, State: contract.TimelineAttemptProposedV1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := proposed.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := contract.AdvanceTimelineProjectionAttemptV1(contract.TimelineAttemptProposedV1, contract.TimelineAttemptVisibleV1); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("proposed skipped admission: %v", err)
	}
	if err := contract.AdvanceTimelineProjectionAttemptV1(contract.TimelineAttemptProposedV1, contract.TimelineAttemptInspectingV1); err != nil {
		t.Fatal(err)
	}
	if err := contract.AdvanceTimelineProjectionAttemptV1(contract.TimelineAttemptInspectingV1, contract.TimelineAttemptAdmittedV1); err != nil {
		t.Fatal(err)
	}
	if err := contract.AdvanceTimelineProjectionAttemptV1(contract.TimelineAttemptAdmittedV1, contract.TimelineAttemptVisibleV1); err != nil {
		t.Fatal(err)
	}
}

func TestTimelineProjectionAttemptV1AdmittedAndVisibleBindings(t *testing.T) {
	request := timelineProjectionRequestV1(t)
	admitted, err := contract.SealTimelineProjectionAttemptV1(contract.TimelineProjectionAttemptFactV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		Ref:             contract.TimelineProjectionAttemptRefV1{AttemptID: request.AttemptID, Revision: 3, ScopeDigest: request.ScopeDigest},
		Request:         request, State: contract.TimelineAttemptAdmittedV1,
		EvidenceProjectionRef: "runtime-projection-1", EvidenceProjectionDigest: "runtime-projection-digest-1",
		EvidenceCurrentIndexRef: "runtime-index-1", EvidenceCurrentIndexDigest: "runtime-index-digest-1",
		PolicyProjectionDigest: "policy-projection-digest-1", CheckedUnixNano: 10, NotAfterUnixNano: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	visible := admitted.Clone()
	visible.Ref.Revision++
	visible.State = contract.TimelineAttemptVisibleV1
	visible.Event = &contract.TimelineEventRefV1{
		EventID: "event-1", EvidenceRecordRef: "evidence-1", LedgerScopeDigest: "ledger-scope-1",
		LedgerSequence: 1, Digest: "event-digest-1",
	}
	visible, err = contract.SealTimelineProjectionAttemptV1(visible)
	if err != nil {
		t.Fatal(err)
	}
	if err := visible.Validate(); err != nil {
		t.Fatal(err)
	}
	drifted := visible.Clone()
	drifted.Event.Digest = "changed"
	if err := drifted.Validate(); !contract.HasCode(err, contract.ErrProjectionConflict) {
		t.Fatalf("visible event drift accepted: %v", err)
	}
}

func timelineProjectionRequestV1(t *testing.T) contract.TimelineProjectionRequestV1 {
	t.Helper()
	request := contract.TimelineProjectionRequestV1{
		ContractVersion: contract.TimelineGovernanceContractVersionV1,
		AttemptID:       "attempt-1", IdempotencyKey: "projection-request-1",
		EvidenceSource:   contract.EvidenceSourceKey{RegistrationID: "source-1", SourceEpoch: 1, SourceSequence: 1},
		ExpectedRecord:   &contract.TimelineEvidenceRecordRefV1{LedgerScopeDigest: "ledger-scope-1", Sequence: 1, RecordDigest: "record-digest-1"},
		ProjectionPolicy: "projection-policy-1", ScopeDigest: "scope-1",
	}
	sealed, err := contract.SealTimelineProjectionRequestV1(request)
	if err != nil {
		t.Fatal(err)
	}
	return sealed
}
