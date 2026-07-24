package contract_test

import (
	"testing"

	"github.com/Proview-China/rax/ExecutionRuntime/continuity/contract"
	"github.com/Proview-China/rax/ExecutionRuntime/continuity/internal/testkit"
)

func TestCheckpointManifestV2ValidatesOwnerFinalizationStates(t *testing.T) {
	for _, state := range []contract.ManifestState{
		contract.ManifestCollecting,
		contract.ManifestVerifiedCandidate,
		contract.ManifestDiagnosticPartial,
		contract.ManifestDiagnosticIndeterminate,
		contract.ManifestRejected,
	} {
		t.Run(string(state), func(t *testing.T) {
			fact := testkit.ManifestV2(state, 1)
			if err := fact.Validate(); err != nil {
				t.Fatalf("valid %s fact rejected: %v", state, err)
			}
			if err := fact.Ref().Validate(); err != nil {
				t.Fatalf("exact ref rejected: %v", err)
			}
		})
	}
}

func TestCheckpointManifestV2RejectsExactRefTamperAndUnknownAsVerified(t *testing.T) {
	verified := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	verified.CheckpointAttemptRef.Digest = "changed-attempt-digest"
	verified.Digest, _ = verified.CanonicalDigest()
	if err := verified.Validate(); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("exact ref tamper with stale frozen-set digest accepted: %v", err)
	}

	unknown := testkit.ManifestV2(contract.ManifestDiagnosticIndeterminate, 2)
	unknown.State = contract.ManifestVerifiedCandidate
	unknown.Digest, _ = unknown.CanonicalDigest()
	if err := unknown.Validate(); !contract.HasCode(err, contract.ErrCheckpointIndeterminate) {
		t.Fatalf("unsettled Begin upgraded to verified_candidate: %v", err)
	}
}

func TestCheckpointManifestV2UnsettledBeginRequiresInspectionAndResidual(t *testing.T) {
	unknown := testkit.ManifestV2(contract.ManifestDiagnosticIndeterminate, 2)
	unknown.AttemptSettlementClosures[0].InspectionRef = nil
	if _, err := unknown.CanonicalDigest(); !contract.HasCode(err, contract.ErrCheckpointIndeterminate) {
		t.Fatalf("unsettled Begin without exact inspection accepted: %v", err)
	}
	unknown = testkit.ManifestV2(contract.ManifestDiagnosticIndeterminate, 2)
	unknown.AttemptSettlementClosures[0].ResidualRefs = nil
	if _, err := unknown.CanonicalDigest(); !contract.HasCode(err, contract.ErrCheckpointIndeterminate) {
		t.Fatalf("unsettled Begin without residual accepted: %v", err)
	}
}

func TestCheckpointManifestV2CanonicalSetsAndCloneNoAlias(t *testing.T) {
	fact := testkit.ManifestV2(contract.ManifestCollecting, 1)
	secondFrame := testkit.ExactRefV2("context-frame-8", "praxis/context", "context-frame")
	fact.ContextFrameRefs = append(fact.ContextFrameRefs, secondFrame)
	fact.MemoryRefs = append(fact.MemoryRefs, testkit.ExactRefV2("memory-watermark-2", "praxis/memory", "memory-watermark"))
	fact.Digest, _ = fact.CanonicalDigest()

	reordered := fact.Clone()
	reordered.ContextFrameRefs[0], reordered.ContextFrameRefs[1] = reordered.ContextFrameRefs[1], reordered.ContextFrameRefs[0]
	reordered.MemoryRefs[0], reordered.MemoryRefs[1] = reordered.MemoryRefs[1], reordered.MemoryRefs[0]
	digest, err := reordered.CanonicalDigest()
	if err != nil || digest != fact.Digest {
		t.Fatalf("canonical set ordering drifted: digest=%s err=%v want=%s", digest, err, fact.Digest)
	}

	clone := fact.Clone()
	clone.ContextFrameRefs[0].ID = "mutated-frame"
	clone.ParticipantClosures[0].EvidenceRefs[0].ID = "mutated-evidence"
	clone.ParticipantClosures[0].SnapshotRef.ID = "mutated-snapshot"
	if fact.ContextFrameRefs[0].ID == "mutated-frame" ||
		fact.ParticipantClosures[0].EvidenceRefs[0].ID == "mutated-evidence" ||
		fact.ParticipantClosures[0].SnapshotRef.ID == "mutated-snapshot" {
		t.Fatal("CheckpointManifestFactV2 Clone leaked aliases")
	}
}

func TestCheckpointManifestV2StateMachineAndSealAreImmutable(t *testing.T) {
	if err := contract.AdvanceCheckpointManifestStateV2(contract.ManifestCollecting, contract.ManifestVerifiedCandidate); err != nil {
		t.Fatalf("collecting finalization rejected: %v", err)
	}
	if err := contract.AdvanceCheckpointManifestStateV2(contract.ManifestVerifiedCandidate, contract.ManifestDiagnosticIndeterminate); !contract.HasCode(err, contract.ErrRevisionConflict) {
		t.Fatalf("terminal state changed: %v", err)
	}

	manifest := testkit.ManifestV2(contract.ManifestVerifiedCandidate, 2)
	seal := testkit.SealV2(manifest)
	if err := seal.Validate(); err != nil {
		t.Fatalf("valid seal rejected: %v", err)
	}
	seal.Revision = 2
	seal.Digest, _ = seal.CanonicalDigest()
	if err := seal.Validate(); !contract.HasCode(err, contract.ErrInvalidArgument) {
		t.Fatalf("non-revision-1 seal accepted: %v", err)
	}
}
