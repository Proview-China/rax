package releasecandidate

// ProofBlockerV1 is a closed, diagnostic-only explanation of why an exact
// production proof is still absent. It never grants production eligibility.
type ProofBlockerV1 string

const (
	BlockerOwnerCurrentCertificationMissingV1 ProofBlockerV1 = "owner_current_certification_missing"
	BlockerRemoteBlobMissingV1                ProofBlockerV1 = "remote_blob_missing"
	BlockerParticipantCaptureMissingV1        ProofBlockerV1 = "participant_capture_missing"
	BlockerRestoreExecuteMissingV1            ProofBlockerV1 = "restore_execute_missing"
	BlockerCleanupRootMissingV1               ProofBlockerV1 = "cleanup_root_missing"
	BlockerDeploymentRootMissingV1            ProofBlockerV1 = "deployment_root_missing"
)

// ProofAssessmentV1 separates durable owner-local implementation evidence
// from an independently certified production proof. Owner-local storage must
// not sign its own production eligibility.
type ProofAssessmentV1 struct {
	Requirement           ProofRequirementV1
	OwnerLocalImplemented bool
	ProductionSatisfied   bool
	Blocker               ProofBlockerV1
}

var proofAssessmentsV1 = []ProofAssessmentV1{
	{Requirement: ProofDurableCheckpointV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofDurableTimelineV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofDurableArtifactV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofDurableHistoryV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofDurableRestoreV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofCurrentIndexesV1, OwnerLocalImplemented: true, Blocker: BlockerOwnerCurrentCertificationMissingV1},
	{Requirement: ProofRemoteBlobV1, Blocker: BlockerRemoteBlobMissingV1},
	{Requirement: ProofParticipantCaptureV1, Blocker: BlockerParticipantCaptureMissingV1},
	{Requirement: ProofRestoreExecuteV1, Blocker: BlockerRestoreExecuteMissingV1},
	{Requirement: ProofCleanupV1, Blocker: BlockerCleanupRootMissingV1},
	{Requirement: ProofDeploymentV1, Blocker: BlockerDeploymentRootMissingV1},
}

func CurrentProofAssessmentsV1() []ProofAssessmentV1 {
	return currentProofAssessmentsV1()
}

func currentProofAssessmentsV1() []ProofAssessmentV1 {
	return append([]ProofAssessmentV1(nil), proofAssessmentsV1...)
}

func sameProofAssessmentsV1(left, right []ProofAssessmentV1) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] || left[index].ProductionSatisfied {
			return false
		}
	}
	return true
}
