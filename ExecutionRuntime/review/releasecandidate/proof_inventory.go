package releasecandidate

// ProofBlockerV1 is a closed, machine-readable explanation for why a required
// production proof is not yet satisfiable. Owner-local software is not itself
// a production-current certification.
type ProofBlockerV1 string

const (
	ProofBlockerOwnerCurrentCertificationMissingV1 ProofBlockerV1 = "owner_current_certification_missing"
	ProofBlockerExternalOwnerRootMissingV1         ProofBlockerV1 = "external_owner_root_missing"
	ProofBlockerRemoteEffectRootMissingV1          ProofBlockerV1 = "remote_effect_root_missing"
	ProofBlockerHumanAdmissionRootMissingV1        ProofBlockerV1 = "human_admission_root_missing"
	ProofBlockerCleanupRootMissingV1               ProofBlockerV1 = "cleanup_root_missing"
	ProofBlockerCompositionRootMissingV1           ProofBlockerV1 = "composition_root_missing"
)

// ProofAssessmentV1 records the current Review release boundary. Implemented
// means Review owns durable/reference software for that slice. It never means
// the production proof is satisfied; that requires an independently published
// exact-current proof and the real composition root.
type ProofAssessmentV1 struct {
	Requirement           ProofRequirementV1
	OwnerLocalImplemented bool
	ProductionSatisfied   bool
	Blocker               ProofBlockerV1
}

func CurrentProofAssessmentsV1() []ProofAssessmentV1 {
	values := currentProofAssessmentsV1()
	return append([]ProofAssessmentV1(nil), values...)
}

func currentProofAssessmentsV1() []ProofAssessmentV1 {
	return []ProofAssessmentV1{
		{Requirement: ProofDecisionCurrentV1, OwnerLocalImplemented: true, Blocker: ProofBlockerOwnerCurrentCertificationMissingV1},
		{Requirement: ProofVerdictCurrentV1, OwnerLocalImplemented: true, Blocker: ProofBlockerOwnerCurrentCertificationMissingV1},
		{Requirement: ProofPolicyCurrentV1, Blocker: ProofBlockerExternalOwnerRootMissingV1},
		{Requirement: ProofEvidenceCurrentV1, Blocker: ProofBlockerExternalOwnerRootMissingV1},
		{Requirement: ProofAuthorityCurrentV1, Blocker: ProofBlockerExternalOwnerRootMissingV1},
		{Requirement: ProofScopeCurrentV1, Blocker: ProofBlockerExternalOwnerRootMissingV1},
		{Requirement: ProofDurableStoreV1, OwnerLocalImplemented: true, Blocker: ProofBlockerOwnerCurrentCertificationMissingV1},
		{Requirement: ProofRemoteEffectV1, Blocker: ProofBlockerRemoteEffectRootMissingV1},
		{Requirement: ProofHumanInterventionV1, OwnerLocalImplemented: true, Blocker: ProofBlockerHumanAdmissionRootMissingV1},
		{Requirement: ProofCleanupV1, Blocker: ProofBlockerCleanupRootMissingV1},
		{Requirement: ProofCompositionRootV1, Blocker: ProofBlockerCompositionRootMissingV1},
	}
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
