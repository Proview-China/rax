package contract_test

import (
	"testing"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/internal/testkit"
)

func TestCheckpointReservationSealBindsTTLAndDeepClones(t *testing.T) {
	participant := testkit.CheckpointParticipant("contract-clone")
	original := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "contract-clone", participant, nil)
	sealed, err := contract.SealCheckpointPhaseReservation(original)
	if err != nil {
		t.Fatal(err)
	}
	original.ChangeSet.Ref.ID = "mutated"
	original.Watermarks[0].Sequence++
	if sealed.ChangeSet.Ref.ID == "mutated" || sealed.Watermarks[0].Sequence != 1 {
		t.Fatal("reservation seal retained caller pointer/slice aliases")
	}

	shorter := sealed
	shorter.Meta.ExpiresUnixNano -= int64(time.Hour)
	shorter, err = contract.SealCheckpointPhaseReservation(shorter)
	if err != nil {
		t.Fatal(err)
	}
	if shorter.Meta.Digest == sealed.Meta.Digest {
		t.Fatal("reservation TTL is absent from canonical digest")
	}
	tampered := shorter
	tampered.Meta.ExpiresUnixNano--
	if err := tampered.ValidateShape(); err == nil {
		t.Fatal("reservation accepted tampered TTL with old digest")
	}
}

func TestCheckpointReservationRequiresExplicitPreviousAndRuntimeCoordinates(t *testing.T) {
	participant := testkit.CheckpointParticipant("contract-shape")
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "contract-shape", participant, nil)

	invalid := reservation
	invalid.PreviousPresence = contract.CheckpointPresent
	if _, err := contract.SealCheckpointPhaseReservation(invalid); err == nil {
		t.Fatal("prepare accepted present previous discriminator without closure")
	}
	invalid = reservation
	invalid.ExpectedParticipantRevision++
	if _, err := contract.SealCheckpointPhaseReservation(invalid); err == nil {
		t.Fatal("participant revision drift was accepted")
	}
	invalid = reservation
	invalid.ExpectedRuntimeAttemptRef = contract.Ref{}
	if _, err := contract.SealCheckpointPhaseReservation(invalid); err == nil {
		t.Fatal("missing expected runtime attempt exact ref was accepted")
	}
	invalid = reservation
	invalid.AttemptID = "caller-minted-attempt"
	if _, err := contract.SealCheckpointPhaseReservation(invalid); err == nil {
		t.Fatal("reservation accepted an attempt ID different from the authoritative checkpoint attempt ref")
	}
	invalid = reservation
	invalid.Runtime.FenceEpoch++
	invalid.Meta.Digest = reservation.Meta.Digest
	if err := invalid.ValidateShape(); err == nil {
		t.Fatal("runtime fence drift retained old digest")
	}
}

func TestCheckpointStableAndBranchKeysIgnoreCallerContentAndClosure(t *testing.T) {
	participant := testkit.CheckpointParticipant("keys")
	first := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "keys-first", participant, nil)
	second := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "keys-second", participant, nil)
	firstKey, _ := contract.CheckpointPhaseKey(first)
	secondKey, _ := contract.CheckpointPhaseKey(second)
	if firstKey != secondKey {
		t.Fatal("stable phase key changed with caller operation/effect content")
	}
	attemptBypass := first
	attemptBypass.AttemptID = "caller-minted-attempt"
	attemptBypassKey, _ := contract.CheckpointPhaseKey(attemptBypass)
	if attemptBypassKey != firstKey {
		t.Fatal("caller-minted attempt ID escaped the authoritative checkpoint attempt phase key")
	}

	closureA := contract.CheckpointPhaseClosureRef{Ref: testkit.Ref("closure-a"), Phase: contract.CheckpointPhasePrepare, State: contract.CheckpointPhasePrepared, ExpiresUnixNano: participant.Meta.ExpiresUnixNano}
	closureB := closureA
	closureB.Ref = testkit.Ref("closure-b")
	commit := testkit.CheckpointReservation(contract.CheckpointPhaseCommit, "keys-commit", participant, &closureA)
	abort := testkit.CheckpointReservation(contract.CheckpointPhaseAbort, "keys-abort", participant, &closureB)
	commitKey, _ := contract.CheckpointBranchKey(commit)
	abortKey, _ := contract.CheckpointBranchKey(abort)
	if commitKey != abortKey {
		t.Fatal("branch key allowed different closure content to select two branches")
	}
	attemptBypass = abort
	attemptBypass.AttemptID = "caller-minted-branch-attempt"
	attemptBypassBranchKey, _ := contract.CheckpointBranchKey(attemptBypass)
	if attemptBypassBranchKey != commitKey {
		t.Fatal("caller-minted attempt ID escaped the authoritative checkpoint attempt branch key")
	}
}

func TestCheckpointReadStageRequiresTypedPresentAndAbsentGates(t *testing.T) {
	participant := testkit.CheckpointParticipant("presence")
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "presence", participant, nil)
	_, request := testkit.CheckpointCurrentFixture(reservation, participant, contract.CheckpointReadPreAdmission)
	if err := request.ValidateShape(); err != nil {
		t.Fatal(err)
	}
	if len(request.ExpectedCurrentRefs) != len(contract.AllCheckpointCurrentKinds()) {
		t.Fatal("request omitted a typed gate")
	}

	missing := request
	missing.ExpectedCurrentRefs = missing.ExpectedCurrentRefs[:len(missing.ExpectedCurrentRefs)-1]
	if err := missing.ValidateShape(); err == nil {
		t.Fatal("request accepted an omitted gate")
	}
	for index, expected := range request.ExpectedCurrentRefs {
		if expected.Kind == contract.CheckpointCurrentAdmission {
			wrong := request
			wrong.ExpectedCurrentRefs = append([]contract.CheckpointExpectedCurrentRef(nil), request.ExpectedCurrentRefs...)
			ref := testkit.Ref("early-admission")
			wrong.ExpectedCurrentRefs[index].Presence = contract.CheckpointPresent
			wrong.ExpectedCurrentRefs[index].Ref = &ref
			if err := wrong.ValidateShape(); err == nil {
				t.Fatal("pre-admission request accepted an early admission gate")
			}
			break
		}
	}
}

func TestCheckpointPhaseFactSealDeepClonesEvidenceAndClosure(t *testing.T) {
	participant := testkit.CheckpointParticipant("fact-clone")
	reservation := testkit.CheckpointReservation(contract.CheckpointPhasePrepare, "fact-clone", participant, nil)
	fact, _ := testkit.CheckpointAppliedPhase(reservation, participant, contract.CheckpointPhaseUnknown, "fact-clone", testkit.FixedNow.Add(time.Hour))
	sealed, err := contract.SealCheckpointPhaseFact(fact)
	if err != nil {
		t.Fatal(err)
	}
	fact.EvidenceRefs[0].ID = "mutated"
	if sealed.EvidenceRefs[0].ID == "mutated" {
		t.Fatal("phase fact seal retained caller evidence slice alias")
	}
}
